package nsocket_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/epikoder/nsocket"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/olahol/melody"
)

const (
	dialAndConnectTimeout = 5 * time.Second
	origin                = "http://localhost:8000/ws"
	url                   = "ws://localhost:8000/ws"
)

func TestNSocket(t *testing.T) {
	authKey := uuid.New().String()
	cookie := []string{"auth=" + authKey}

	mux := http.NewServeMux()

	// Configure Nsocket server
	soc := nsocket.New(nsocket.Config{
		AuthFunc: func(r *http.Request) (ok bool) {
			c, err := r.Cookie("auth")
			if err != nil {
				return
			}
			return c.Value == authKey
		},
		AllowedOrigins: []string{"localhost:3000", "localhost:8000"},
		Namespace: nsocket.Namespace{
			nsocket.Default: nsocket.Event{
				"/": func(s *melody.Session, i interface{}, soc *nsocket.NSocket) {
					fmt.Printf("Namespace: [Default] -- GOT: %v ---- from ----  %v\n", i, s.RemoteAddr())
					if err := soc.Broadcast("namespace:default -- " + fmt.Sprintf("%v ------> %v", i, s.RemoteAddr())); err != nil {
						t.Error(err)
					}
				},
				"message": func(s *melody.Session, i interface{}, soc *nsocket.NSocket) {
					fmt.Printf("Namespace: [Default/Message] -- GOT: %v ---- from ----  %v\n", i, s.RemoteAddr())
					if err := soc.Emit("namespace:message -- "+fmt.Sprintf("%v ------> %v", i, s.RemoteAddr()), "message"); err != nil {
						t.Error(err)
					}
				},
			},
		},
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if err := soc.Serve(w, r); err != nil {
			t.Error(err)
		}
	})

	fmt.Println("starting server on: http://localhost:8000")
	go http.ListenAndServe(":8000", mux)

	type Client = struct {
		Ctx    context.Context
		Cancel context.CancelFunc
		Conn   *websocket.Conn
	}
	var client Client
	var wg sync.WaitGroup

	var reqHeader http.Header = http.Header{
		"Cookie": cookie,
	}
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(dialAndConnectTimeout))
	conn, _, err := websocket.DefaultDialer.Dial(url, reqHeader)
	if err != nil {
		panic(err)
	}
	client = Client{
		ctx, cancel, conn,
	}
	client.Conn.EnableWriteCompression(true)
	go func() {
		for {
			var v interface{}
			// err := conn.ReadJSON(&v)
			_, b, err := client.Conn.ReadMessage()
			if err != nil && err != io.EOF {
				panic(err)
			}
			if err = json.Unmarshal(b, &v); err != nil {
				fmt.Println(err)
			}
			fmt.Printf("Client:: - %v \n", v)
		}
	}()

	wg.Add(1)
	client.Conn.WriteJSON(map[string]interface{}{
		"id":        uuid.New(),
		"type":      "nsocket",
		"action":    "subscribe",
		"namespace": "message",
	})

	wg.Add(1)
	client.Conn.WriteJSON(map[string]interface{}{
		"id":        uuid.New(),
		"type":      "emit",
		"body":      "I should not be received",
		"namespace": "not-found",
	})

	wg.Add(1)
	client.Conn.WriteJSON(map[string]interface{}{
		"id":        uuid.New(),
		"type":      "emit",
		"body":      "I should be received in namspace::message",
		"namespace": "message",
	})

	wg.Add(1)
	client.Conn.WriteJSON(map[string]interface{}{
		"id":   uuid.New(),
		"type": "emit",
		"body": "I should be received in namspace::default",
	})

	wg.Add(1)
	client.Conn.WriteJSON(map[string]interface{}{
		"id":        uuid.New(),
		"type":      "nsocket",
		"action":    "unsubscribe",
		"namespace": "message",
	})
	time.Sleep(time.Second * 2)
}
