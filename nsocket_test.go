package nsocket_test

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/epikoder/nsocket"
	"github.com/gorilla/websocket"
	"github.com/olahol/melody"
)

const (
	dialAndConnectTimeout = 5 * time.Second
	origin                = "http://localhost:8000/ws"
	url                   = "ws://localhost:8000/ws"
)

func TestTnny(t *testing.T) {
	type Client = struct {
		Ctx    context.Context
		Cancel context.CancelFunc
		Conn   *websocket.Conn
		Status bool
	}
	var client1, client2, client3 Client

	mux := http.NewServeMux()
	soc := nsocket.New(nsocket.Config{
		Namespace: nsocket.Namespace{
			nsocket.Default: nsocket.Event{
				"/": func(s *melody.Session, i interface{}, soc *nsocket.NSocket) {
					fmt.Printf("GOT: %v from default/%v\n", i, s.RemoteAddr())
					if err := soc.Emit(map[string]interface{}{
						"message": "ROOT:::" + fmt.Sprintf("%v", i),
					}, "/"); err != nil {
						t.Error(err)
					}
				},
				"message": func(s *melody.Session, i interface{}, soc *nsocket.NSocket) {
					fmt.Printf("GOT::::: %v from default/%v\n", i, s.RemoteAddr())
					if err := soc.Emit(map[string]interface{}{
						"message": "MESSAGE:::" + fmt.Sprintf("%v", i),
					}, "message"); err != nil {
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

	{
		ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(dialAndConnectTimeout))
		conn, _, err := websocket.DefaultDialer.Dial(url, nil)
		if err != nil {
			panic(err)
		}
		client1 = Client{
			ctx, cancel, conn, true,
		}
		// go func() {
		// 	for {
		// 		if client1.Status {
		// 			v := map[string]interface{}{}
		// 			err := conn.ReadJSON(&v)
		// 			fmt.Println("CLIENT1 : ", v, err)
		// 		}
		// 		time.Sleep(time.Second * 1)
		// 	}
		// }()
	}
	{
		ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(dialAndConnectTimeout))
		conn, _, err := websocket.DefaultDialer.Dial(url, nil)
		if err != nil {
			panic(err)
		}
		client2 = Client{
			ctx, cancel, conn, true,
		}
		// go func() {
		// 	for {
		// 		if client2.Status {
		// 			v := map[string]interface{}{}
		// 			err := conn.ReadJSON(&v)
		// 			fmt.Println("CLIENT2 : ", v, err)
		// 		}
		// 		time.Sleep(time.Second * 1)
		// 	}
		// }()
	}
	{
		ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(dialAndConnectTimeout))
		conn, _, err := websocket.DefaultDialer.Dial(url, nil)
		if err != nil {
			panic(err)
		}
		client3 = Client{
			ctx, cancel, conn, true,
		}
		// go func() {
		// 	for {
		// 		if client3.Status {
		// 			v := map[string]interface{}{}
		// 			err := conn.ReadJSON(&v)
		// 			fmt.Println("CLIENT3 : ", v, err)
		// 		}
		// 		time.Sleep(time.Second * 1)
		// 	}
		// }()
	}
	clients := []Client{client1, client2, client3}

	for i, c := range clients {
		if i == 2 {
			continue
		}
		c.Conn.WriteJSON(map[string]interface{}{
			"type":      "iosoc",
			"action":    "subscribe",
			"namespace": "message",
		})
	}

	time.Sleep(time.Second * 2)
	for _, c := range clients {
		c.Conn.WriteJSON(map[string]interface{}{
			"type": "emit",
			"body": "I should be received",
		})
	}
	for _, c := range clients {
		c.Conn.WriteJSON(map[string]interface{}{
			"type":      "emit",
			"body":      "I should not be received",
			"namespace": "not-found",
		})
	}
	for _, c := range clients {
		c.Conn.WriteJSON(map[string]interface{}{
			"type":      "emit",
			"body":      "I should be received in message",
			"namespace": "message",
		})
	}

	time.Sleep(time.Second * 5)
	for i, c := range clients {
		c.Conn.WriteMessage(1, []byte(fmt.Sprintf("Hello from client %d", i+1)))
		c.Conn.WriteJSON(map[string]interface{}{
			"type":      "iosoc",
			"action":    "unsubscribe",
			"namespace": "message",
		})
	}

	time.Sleep(time.Second * 1)
	for _, c := range clients {
		c.Status = false
		c.Conn.Close()
	}
	time.Sleep(time.Second * 2)
}
