package main

import (

    "fmt"
	"net/http"
    "runtime"
    "time"
    "../../websocket-go"
)



var server *websocket.Server  // with the default configuration


func handleWebsocketConnection(c *websocket.Connection) {



	c.Set("test", "test value")

	c.Join("testroom")

    fmt.Println(c.List("testroom"))

    c.OnMessage(func(bytes []byte){

        c.Emit("chat",string(bytes))
    })

	c.On("chat", func(message string) {

		c.To("testroom").Emit("chat", "fafafafafa")
	})

	c.OnDisconnect(func() {

    })


    go func(){        

        server.Of("testnamespace").To("testroom").Emit("chat","fffffffffffffffff")

    }()

}


func CustomConnecionID(req *http.Request) string {
    
    id := fmt.Sprintf("%s%d",req.URL.Path, time.Now().UnixNano())

    return id
}

func main() {
    
    num := runtime.NumCPU()
    runtime.GOMAXPROCS(num)

    server = websocket.New(websocket.Config{
        CustomIDFunc:CustomConnecionID,
    })

	http.Handle("/testnamespace", server.Handler())

	server.OnConnection(handleWebsocketConnection)

	server.Serve()

	fmt.Println("start server ", "3000")

	http.ListenAndServe(":3000", nil)

}
