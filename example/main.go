package main

import (

    "log"
    "fmt"
	"net/http"
    "runtime"
    "time"
    
    "../../websocket-go"
)



var server *websocket.Server  // with the default configuration


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


    // test room
    go func(c *websocket.Connection){

        time.Sleep(2 * time.Second)

        c.To("testroom").Emit("chat","fffffffff")

        log.Printf("rooms  %v", c.List("testroom"))

    }(c)

    // test namespace
    go func(){
        
        time.Sleep(4 * time.Second)

        server.Of("testnamespace").To("testroom").Emit("chat","fffffffffffffffff")

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
