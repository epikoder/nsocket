# NSocket
#### Messaging framework with native Websocket under the hood

Inspired by [Socket.io](https://socket.io)

### Installation
```
go get github.com/epikoder/nsocket
```

### Usage

```
events := nsocket.Event{
				"/": func(s *melody.Session, i interface{}, soc *nsocket.NSocket) {
					fmt.Printf("GOT: %v from default/%v\n", i, s.RemoteAddr())
					if err := soc.Broadcast(map[string]interface{}{                          // Send a response to all clients
						"message": "ROOT EVENT:::" + fmt.Sprintf("%v ------> %v", i, s.RemoteAddr()),
					}, "/"); err != nil {
            fmt.Println(err)
					}
				},
				"message": func(s *melody.Session, i interface{}, soc *nsocket.NSocket) {
					fmt.Printf("GOT::::: %v from default/%v\n", i, s.RemoteAddr())
					if err := soc.Emit(map[string]interface{}{                                // Send a response to only message channel
						"message": "MESSAGE CHANNEL:::" + fmt.Sprintf("%v ------> %v", i, s.RemoteAddr()),
					}, "message"); err != nil {
            fmt.Println(err)
					}
				},
		}
    
soc := nsocket.New(nsocket.Config{
		AuthFunc: func(r *http.Request) (ok bool) { // Optional: Add authentication : On https Websocket send cookie to the server
			c, err := r.Cookie("auth")
			if err != nil {
				return
			}
			return c.Value == authKey
		},
		AllowedOrigins: []string{"localhost:3000", "localhost:8000"}, //Optional: Set allowed origins if needed
		Namespace: nsocket.Namespace{
			nsocket.Default: events // Events functions are called when message is received from the client
		},
	})
```

##### Note: 
This library is still under development and not production ready, use with caution.
