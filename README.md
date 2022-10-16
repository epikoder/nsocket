# NSocket

## Messaging framework with channels [rooms]

Inspired by [Socket.io](https://socket.io) and [Kataras Neffos](https://github.com/kataras/neffos)

### Installation

```bash
go get github.com/epikoder/nsocket
```

### Usage

```go
events := nsocket.Event{
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

 mux := http.NewServeMux()
 mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
  if err := soc.Serve(w, r); err != nil {
   panic(err)
  }
 })

 fmt.Println("starting server on: http://localhost:8000")
 go http.ListenAndServe(":8000", mux)
```

### Note

This library is still under development and not production ready, use with caution.
