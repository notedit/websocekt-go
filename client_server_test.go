package websocket

import (
	"net/http"
	"testing"
	"time"

	ws "github.com/gorilla/websocket"
)

var server = New(Config{})
var addr = "localhost:8080"

func runServer() {

	http.Handle("/testnamespace", server.Handler())

	server.OnConnection(func(c *Connection) {

		c.Join("room")

		c.OnMessage(func(bytes []byte) {
			c.EmitMessage(bytes)
		})

		c.OnDisconnect(func() {
			c.Leave("room")
		})

	})

	server.Serve()

	http.ListenAndServe(addr, nil)

}

func TestRoomJoinLeave(t *testing.T) {

	go runServer()

	_, _, err := ws.DefaultDialer.Dial("ws://"+addr+"/testnamespace", nil)

	if err != nil {
		t.Error("client can not connect")
	}

	time.Sleep(time.Second * 1)

	connlist := server.Of("testnamespace").List("room")

	if len(connlist) != 1 {
		t.Error("join room fail")
	}

	c2, _, err := ws.DefaultDialer.Dial("ws://"+addr+"/testnamespace", nil)

	if err != nil {
		t.Error("client can not connect")
	}

	time.Sleep(time.Second * 1)

	connlist = server.Of("testnamespace").List("room")

	if len(connlist) != 2 {
		t.Error("join room fail2")
	}

	c2.Close()

	time.Sleep(time.Second * 1)

	connlist = server.Of("testnamespace").List("room")

	if len(connlist) != 1 {

		t.Error("leave room fail")
	}

	server.Stop()

}

func BenchmarkLoops(b *testing.B) {

	go runServer()

}
