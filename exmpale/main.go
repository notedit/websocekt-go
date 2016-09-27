package main

import (
	"fmt"
	"net/http"

    
    "github.com/notedit/websocket-go"
)

func handleWebsocketConnection(c *websocket.Connection) {

	fmt.Printf("request: %v\n", c.Request())

	c.Set("test", "test value")

	c.Join("room")

	c.On("chat", func(message string) {

		c.To("room").Emit("chat", "fafafafafa")
	})

	c.OnDisconnect(func() {
		fmt.Printf("\nConnection with ID: %s has been disconnected!", c.ID())
	})
}

func main() {

	ws := websocket.New(websocket.Config{}) // with the default configuration

	http.Handle("/ws", ws.Handler())

	ws.OnConnection(handleWebsocketConnection)

	ws.Serve()

	fmt.Println("start server ", "3000")

	http.ListenAndServe(":3000", nil)

}
