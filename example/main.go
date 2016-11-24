package main

import (
	"fmt"
	"net/http"
	"runtime"

	"../../websocket-go/server"
)

var server *websocket.Server // with the default configuration

func handleWebsocketConnection(c *websocket.Connection) {

	c.Set("test", "test value")

	c.Join("testroom")

	c.List("testroom")

	c.OnMessage(func(bytes []byte) {

	})

	c.On("chat", func(message string) {

		c.To("testroom").Emit("chat", "fafafafafa")
	})

	c.OnDisconnect(func() {

	})

	go func() {

		server.Of("testnamespace").To("testroom").Emit("chat", "fffffffffffffffff")

	}()

}

func main() {

	num := runtime.NumCPU()
	runtime.GOMAXPROCS(num)

	server = websocket.New(websocket.Config{})

	http.Handle("/testnamespace", server.Handler())

	server.OnConnection(handleWebsocketConnection)

	server.Serve()

	fmt.Println("start server ", "3000")

	http.ListenAndServe(":3000", nil)

}
