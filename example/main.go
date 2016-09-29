package main

import (

    "log"
    "fmt"
	"net/http"

    "time"
    
    "../../websocket-go"
)

func handleWebsocketConnection(c *websocket.Connection) {


    log.Println("handleWebsocketConnection")

	c.Set("test", "test value")

	c.Join("testroom")

    c.List("testroom")

	c.On("chat", func(message string) {

		c.To("testroom").Emit("chat", "fafafafafa")
	})

	c.OnDisconnect(func() {
		fmt.Printf("\nConnection with ID: %s has been disconnected!", c.ID())
	})


    go func(c *websocket.Connection){

        time.Sleep(2 * time.Second)

        c.To("testroom").Emit("chat","fffffffff")

        log.Printf("rooms  %v", c.List("testroom"))

    }(c)

}

func main() {

	ws := websocket.New(websocket.Config{}) // with the default configuration

	http.Handle("/ws", ws.Handler())

	ws.OnConnection(handleWebsocketConnection)

	ws.Serve()

	fmt.Println("start server ", "3000")

	http.ListenAndServe(":3000", nil)

}
