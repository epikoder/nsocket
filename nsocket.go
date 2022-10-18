package nsocket

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"

	"github.com/google/uuid"
	"github.com/olahol/melody"
)

type (
	iNSocket interface {
		Emit(v interface{}, namespace string) error
		Broadcast(v interface{}, namespace string) error
	}

	NSocket struct {
		rwMutex    sync.RWMutex
		melody     *melody.Melody
		namespaces map[string][]*melody.Session
		config     Config
		iNSocket
	}
	SendFunc func(interface{}, string) error
	RecFunc  func(*melody.Session, interface{}, *NSocket)

	Namespace map[string]Event
	Event     map[string]RecFunc

	Config struct {
		AllowedOrigins []string
		AuthFunc       func(r *http.Request) bool
		Namespace      Namespace
	}
)

type (
	Message struct {
		Id        uuid.UUID     `json:"id"`
		Type      MessageType   `json:"type"`
		Body      interface{}   `json:"body"`
		Action    MessageAction `json:"action,omitempty"`
		Namespace string        `json:"namespace,omitempty"`
	}

	MessageType   string
	MessageAction string
)

const (
	_NSOCKET_ MessageType = "nsocket"
	_EMIT_    MessageType = "emit"

	Nil         MessageAction = ""
	Subscribe   MessageAction = "subscribe"
	UnSubscribe MessageAction = "unsubscribe"

	OnNamespaceConnected  = "OnNamespaceConnected"
	OnNamespaceDisconnect = "OnNamespaceDisconnect"
)

const (
	Default string = "default"
)

var (
	DefaultNSocketConfig Config = Config{}
)

func (nsoc *NSocket) onMessage(namespace string, sess *melody.Session, message interface{}) {
	events := nsoc.config.Namespace[Default]
	f, ok := events[resolveNamespace(namespace)]
	if !ok {
		return
	}
	f(sess, message, nsoc)
}

type AckMessage struct {
	Id     uuid.UUID `json:"id"`
	Type   string    `json:"type"`
	Action string    `json:"action"`
	Reason string    `json:"reason,omitempty"`
}

func (nsoc *NSocket) ackRespond(s *melody.Session, m AckMessage) {
	if b, err := json.Marshal(m); err == nil {
		s.Write(b)
	}
}

func (nsoc *NSocket) subscribe(namespace string, sess *melody.Session) {
	nsoc.rwMutex.Lock()
	events := nsoc.config.Namespace[Default]
	if _, ok := events[namespace]; !ok {
		return
	}
	_, ok := nsoc.namespaces[namespace]
	if !ok {
		nsoc.namespaces[namespace] = []*melody.Session{}
	}
	nsoc.namespaces[namespace] = append(nsoc.namespaces[namespace], sess)
	nsoc.rwMutex.Unlock()
}

func (nsoc *NSocket) unsubscribe(namespace string, sess *melody.Session) {
	events := nsoc.config.Namespace[Default]
	if _, ok := events[namespace]; !ok {
		return
	}

	nsoc.rwMutex.Lock()
	_socs, ok := nsoc.namespaces[namespace]
	if !ok {
		return
	}
	tmp := []*melody.Session{}
	for _, s := range _socs {
		if s != sess {
			tmp = append(tmp, s)
		}
	}
	nsoc.namespaces[namespace] = tmp
	nsoc.rwMutex.Unlock()
}

func (nsoc *NSocket) unsubscribeFromAll(sess *melody.Session) {
	nsoc.rwMutex.Lock()
	for i, v := range nsoc.namespaces {
		tmp := []*melody.Session{}
		for _, s := range v {
			if s == sess || s.IsClosed() {
				s.Close()
				continue
			}
			tmp = append(tmp, s)
		}
		nsoc.namespaces[i] = tmp
	}
	nsoc.rwMutex.Unlock()
}

func New(config Config) *NSocket {
	nsoc := &NSocket{
		rwMutex: sync.RWMutex{},
		melody:  melody.New(),
		namespaces: map[string][]*melody.Session{
			Default: {},
		},
	}
	nsoc.melody.HandleMessage(func(s *melody.Session, b []byte) {
		message := Message{}
		err := json.Unmarshal(b, &message)
		if err != nil {
			nsoc.ackRespond(s, AckMessage{
				Id:     uuid.New(),
				Type:   "ack",
				Action: "failed",
				Reason: err.Error(),
			})
			return
		}

		switch message.Type {
		case _NSOCKET_:
			{
				switch message.Action {
				case Subscribe:
					nsoc.subscribe(resolveNamespace(message.Namespace), s)
				case UnSubscribe:
					nsoc.unsubscribe(resolveNamespace(message.Namespace), s)
				}
			}
		case _EMIT_:
			{
				nsoc.onMessage(message.Namespace, s, message.Body)
				nsoc.ackRespond(s, AckMessage{
					Id:     message.Id,
					Type:   "ack",
					Action: "received",
				})
			}
		}
	})

	nsoc.melody.HandleConnect(func(s *melody.Session) {
		fmt.Printf("Connected to :%v\n", s.RemoteAddr())
		events := nsoc.config.Namespace[Default]
		f, ok := events[resolveNamespace(OnNamespaceConnected)]
		if !ok {
			return
		}
		f(s, nil, nsoc)
	})

	nsoc.melody.HandleDisconnect(func(s *melody.Session) {
		fmt.Printf("Client :%v Disconnected -- %s \n", s.RemoteAddr(), s.Request.Context().Err())
		nsoc.unsubscribeFromAll(s)
		events := nsoc.config.Namespace[Default]
		f, ok := events[resolveNamespace(OnNamespaceDisconnect)]
		if !ok {
			return
		}
		f(s, nil, nsoc)
	})

	if config.AllowedOrigins != nil {
		nsoc.melody.Upgrader.CheckOrigin = func(r *http.Request) (ok bool) {
			origin := r.Header.Get("origin")
			if ok = origin == ""; !ok {
				if arr := strings.Split(origin, "//"); len(arr) == 2 {
					for _, o := range config.AllowedOrigins {
						if ok = o == arr[1]; ok {
							return
						}
					}
				}
			}
			return
		}
	}
	nsoc.config = config
	tmp := config.Namespace[Default]
	events := Event{}
	for namespace, f := range tmp {
		ns := resolveNamespace(namespace)
		nsoc.namespaces[ns] = []*melody.Session{}
		events[ns] = f
	}
	config.Namespace[Default] = events
	return nsoc
}

func (nsoc *NSocket) Emit(v interface{}, i ...interface{}) (err error) {
	var namespace string
	var s *melody.Session

	for k := 0; k < 2; k++ {
		switch i[k].(type) {
		case *melody.Session:
			s = i[k].(*melody.Session)
		case string:
			namespace = i[k].(string)
		}
	}
	nsoc.rwMutex.Lock()
	ns := resolveNamespace(namespace)
	sess, ok := nsoc.namespaces[ns]
	if !ok {
		return fmt.Errorf("namespace not found")
	}
	nsoc.rwMutex.Unlock()
	if s != nil {
		tmp := sess
		sess = []*melody.Session{}
		for _, ss := range tmp {
			if ss != s {
				sess = append(sess, s)
			}
		}
	}
	var b []byte
	if v != nil {
		b, err = json.Marshal(Message{
			Id:        uuid.New(),
			Type:      _EMIT_,
			Body:      v,
			Namespace: namespace,
		})
		if err != nil {
			return
		}
	}
	return nsoc.melody.BroadcastMultiple(b, sess)
}

func (nsoc *NSocket) EmitAll(v interface{}, namespace string) (err error) {
	nsoc.rwMutex.Lock()
	ns := resolveNamespace(namespace)
	sess, ok := nsoc.namespaces[ns]
	if !ok {
		return fmt.Errorf("namespace not found")
	}
	nsoc.rwMutex.Unlock()
	var b []byte
	if v != nil {
		b, err = json.Marshal(Message{
			Id:        uuid.New(),
			Type:      _EMIT_,
			Body:      v,
			Namespace: namespace,
		})
		if err != nil {
			return
		}
	}
	return nsoc.melody.BroadcastMultiple(b, sess)
}

func (nsoc *NSocket) Broadcast(v interface{}, s *melody.Session) (err error) {
	var b []byte
	if v != nil {
		b, err = json.Marshal(v)
		if err != nil {
			return
		}
	}
	return nsoc.melody.BroadcastOthers(b, s)
}

func (nsoc *NSocket) BroadcastAll(v interface{}) (err error) {
	var b []byte
	if v != nil {
		b, err = json.Marshal(v)
		if err != nil {
			return
		}
	}
	return nsoc.melody.Broadcast(b)
}

func (nsoc *NSocket) Namespaces() map[string][]*melody.Session {
	return nsoc.namespaces
}

func (nsoc *NSocket) Serve(w http.ResponseWriter, r *http.Request) (err error) {
	if nsoc.config.AuthFunc != nil && !nsoc.config.AuthFunc(r) {
		w.WriteHeader(401)
		return
	}
	if err = nsoc.melody.HandleRequest(w, r); err != nil {
		return
	}
	return
}
