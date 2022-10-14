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
		Namespaces map[string][]*melody.Session
		config     Config
		iNSocket
	}
	SendFunc func(interface{}, string) error
	RecFunc  func(*melody.Session, interface{}, *NSocket)

	Namespace map[string]Event
	Event     map[string]RecFunc

	Config struct {
		AllowedOrigins []string
		Namespace      Namespace
	}
)

type (
	Message struct {
		Id        uuid.UUID
		Type      MessageType
		Body      interface{}
		Action    MessageAction
		Namespace string
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
	// OnNamespaceClose      = "OnNamespaceClose"
)

const (
	Default string = "default"
)

var (
	nsoc *NSocket = &NSocket{
		rwMutex: sync.RWMutex{},
		melody:  melody.New(),
		Namespaces: map[string][]*melody.Session{
			Default: {},
		},
	}
	DefaultNSocketConfig Config = Config{}
)

func init() {
	nsoc.melody.HandleMessage(func(s *melody.Session, b []byte) {
		message := Message{}
		err := json.Unmarshal(b, &message)
		if err != nil {
			return
		}

		switch message.Type {
		case _NSOCKET_:
			{
				switch message.Action {
				case Subscribe:
					subscribe(getNamespace(message.Namespace), s)
				case UnSubscribe:
					unsubscribe(getNamespace(message.Namespace), s)
				}
			}
		case _EMIT_:
			{
				clientEmit(message.Namespace, s, message.Body)
				respondEmit(s, message.Id)
			}
		}
	})

	nsoc.melody.HandleConnect(func(s *melody.Session) {
		fmt.Printf("Connected to :%v\n", s.RemoteAddr())
		events := nsoc.config.Namespace[Default]
		f, ok := events[OnNamespaceConnected]
		if !ok {
			return
		}
		f(s, nil, nsoc)
	})

	nsoc.melody.HandleDisconnect(func(s *melody.Session) {
		fmt.Printf("Client :%v Disconnected\n", s.RemoteAddr())
		events := nsoc.config.Namespace[Default]
		f, ok := events[OnNamespaceDisconnect]
		if !ok {
			return
		}
		f(s, nil, nsoc)
	})
}

func clientEmit(namespace string, sess *melody.Session, message interface{}) {
	events := nsoc.config.Namespace[Default]
	f, ok := events[getNamespace(namespace)]
	if !ok {
		return
	}
	f(sess, message, nsoc)
}

func respondEmit(s *melody.Session, id uuid.UUID) {
	if b, err := json.Marshal(struct {
		Id     uuid.UUID
		Action string
	}{id, "received"}); err == nil {
		s.Write(b)
	}
}

func subscribe(namespace string, sess *melody.Session) {
	events := nsoc.config.Namespace[Default]
	if _, ok := events[namespace]; !ok {
		return
	}

	nsoc.rwMutex.Lock()
	_, ok := nsoc.Namespaces[namespace]
	if !ok {
		nsoc.Namespaces[namespace] = []*melody.Session{}
	}
	nsoc.Namespaces[namespace] = append(nsoc.Namespaces[namespace], sess)
	nsoc.rwMutex.Unlock()
}

func unsubscribe(namespace string, sess *melody.Session) {
	events := nsoc.config.Namespace[Default]
	if _, ok := events[namespace]; !ok {
		return
	}

	nsoc.rwMutex.Lock()
	_socs, ok := nsoc.Namespaces[namespace]
	if !ok {
		return
	}
	tmp := []*melody.Session{}
	for _, s := range _socs {
		if s != sess {
			tmp = append(tmp, s)
		}
	}
	nsoc.Namespaces[namespace] = tmp
	nsoc.rwMutex.Unlock()
}

func New(config Config) *NSocket {
	if config.AllowedOrigins != nil {
		nsoc.melody.Upgrader.CheckOrigin = func(r *http.Request) (ok bool) {
			if arr := strings.Split(r.Header.Get("origin"), "//"); len(arr) == 2 {
				for _, o := range config.AllowedOrigins {
					if ok = o == arr[1]; ok {
						return
					}
				}
			}
			return
		}
	}
	nsoc.config = config
	events := config.Namespace[Default]
	for namespace, f := range events {
		namespace = getNamespace(namespace)
		nsoc.Namespaces[namespace] = []*melody.Session{}
		events[namespace] = f
	}
	config.Namespace[Default] = events
	return nsoc
}

func (*NSocket) Emit(v interface{}, namespace string) (err error) {
	namespace = getNamespace(namespace)
	nsoc.rwMutex.Lock()
	sess, ok := nsoc.Namespaces[namespace]
	if !ok {
		return fmt.Errorf("namespace not found")
	}
	nsoc.rwMutex.Unlock()
	var b []byte
	if v != nil {
		b, err = json.Marshal(v)
		if err != nil {
			return
		}
	}
	return nsoc.melody.BroadcastMultiple(b, sess)
}

func (*NSocket) Broadcast(v interface{}, namespace string) (err error) {
	var b []byte
	if v != nil {
		b, err = json.Marshal(v)
		if err != nil {
			return
		}
	}
	return nsoc.melody.Broadcast(b)
}

func (*NSocket) Serve(w http.ResponseWriter, r *http.Request) (err error) {
	if err = nsoc.melody.HandleRequest(w, r); err != nil {
		return
	}
	return
}

func getNamespace(namespace string) (s string) {
	if namespace == Default {
		return Default
	}
	if namespace == "/" || namespace == "" {
		return Default
	}
	s = strings.TrimPrefix(namespace, "/")
	s = strings.TrimSuffix(s, "/")
	s = Default + "/" + s
	return
}
