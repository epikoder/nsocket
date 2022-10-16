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
			arkRespond(s, ArkMessage{
				Id:     uuid.New(),
				Type:   "ark",
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
					subscribe(genNamespace(message.Namespace), s)
				case UnSubscribe:
					unsubscribe(genNamespace(message.Namespace), s)
				}
			}
		case _EMIT_:
			{
				onMessage(message.Namespace, s, message.Body)
				arkRespond(s, ArkMessage{
					Id:     message.Id,
					Type:   "ark",
					Action: "received",
				})
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

func onMessage(namespace string, sess *melody.Session, message interface{}) {
	events := nsoc.config.Namespace[Default]
	f, ok := events[genNamespace(namespace)]
	if !ok {
		return
	}
	f(sess, message, nsoc)
}

type ArkMessage struct {
	Id     uuid.UUID `json:"id"`
	Type   string    `json:"type"`
	Action string    `json:"action"`
	Reason string    `json:"reason,omitempty"`
}

func arkRespond(s *melody.Session, m ArkMessage) {
	if b, err := json.Marshal(m); err == nil {
		s.Write(b)
	}
}

func subscribe(namespace string, sess *melody.Session) {
	nsoc.rwMutex.Lock()
	events := nsoc.config.Namespace[Default]
	if _, ok := events[namespace]; !ok {
		return
	}
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
		ns := genNamespace(namespace)
		nsoc.Namespaces[ns] = []*melody.Session{}
		events[ns] = f
	}
	config.Namespace[Default] = events
	return nsoc
}

func (*NSocket) Emit(v interface{}, namespace string) (err error) {
	nsoc.rwMutex.Lock()
	ns := genNamespace(namespace)
	sess, ok := nsoc.Namespaces[ns]
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
	if nsoc.config.AuthFunc != nil && !nsoc.config.AuthFunc(r) {
		w.WriteHeader(401)
		return
	}
	if err = nsoc.melody.HandleRequest(w, r); err != nil {
		return
	}
	return
}

func genNamespace(namespace string) (s string) {
	if namespace == Default {
		return Default
	}
	if namespace == "/" || namespace == "" {
		return Default
	}
	s = strings.TrimPrefix(strings.TrimPrefix(namespace, Default), "/")
	s = strings.TrimSuffix(s, "/")
	s = Default + "/" + s
	return
}
