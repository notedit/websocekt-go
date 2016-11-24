package websocket

import (
	"bytes"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type (
	// DisconnectFunc is the callback which fires when a client/connection closed
	DisconnectFunc func()
	// ErrorFunc is the callback which fires when an error happens
	ErrorFunc (func(string))
	// NativeMessageFunc is the callback for native websocket messages, receives one []byte parameter which is the raw client's message
	NativeMessageFunc func([]byte)
	// MessageFunc is the second argument to the Emmiter's Emit functions.
	// A callback which should receives one parameter of type string, int, bool or any valid JSON/Go struct
	MessageFunc interface{}

	ConnectionIDFunc func(*http.Request) string

	// Connection is the front-end API that you will use to communicate with the client side

	Connection struct {
		underline                *websocket.Conn
		id                       string
		messageType              int
		send                     chan []byte
		onDisconnectListeners    []DisconnectFunc
		onErrorListeners         []ErrorFunc
		onNativeMessageListeners []NativeMessageFunc
		onEventListeners         map[string][]MessageFunc

		request *http.Request
		server  *Server

		namespace *NameSpace

		//add some custom data
		data map[string]string
	}
)

func newConnection(underlineConn *websocket.Conn, s *Server, req *http.Request) *Connection {
	c := &Connection{
		underline:   underlineConn,
		messageType: websocket.TextMessage,
		send:        make(chan []byte, 256),
		onDisconnectListeners:    make([]DisconnectFunc, 0),
		onErrorListeners:         make([]ErrorFunc, 0),
		onNativeMessageListeners: make([]NativeMessageFunc, 0),
		onEventListeners:         make(map[string][]MessageFunc, 0),
		server:                   s,
		request:                  req,
		data:                     make(map[string]string),
	}

	if s.config.CustomIDFunc != nil {
		c.id = s.config.CustomIDFunc(req)
	} else {
		c.id = RandomString(64)
	}

	fmt.Println(c.id)

	namespaceName := c.Request().URL.Path

	if strings.HasPrefix(namespaceName, "/") {
		namespaceName = namespaceName[1:]
	}

	s.nsLock.Lock()
	namespace, ok := s.namespaces[namespaceName]

	if !ok {
		namespace = &NameSpace{server: s, name: namespaceName, rooms: make(Rooms), mu: sync.Mutex{}}
		s.namespaces[namespaceName] = namespace
	}
	s.nsLock.Unlock()

	c.namespace = namespace

	if s.config.BinaryMessages {
		c.messageType = websocket.TextMessage
	}

	return c
}

func (c *Connection) write(websocketMessageType int, data []byte) error {
	c.underline.SetWriteDeadline(time.Now().Add(c.server.config.WriteTimeout))
	return c.underline.WriteMessage(websocketMessageType, data)
}

func (c *Connection) writer() {
	ticker := time.NewTicker(c.server.config.PingPeriod)
	defer func() {
		ticker.Stop()
		c.Disconnect()
	}()

	for {
		select {
		case msg, ok := <-c.send:
			if !ok {
				defer func() {
					if err := recover(); err != nil {
						ticker.Stop()
						c.server.free <- c
						c.underline.Close()
					}
				}()
				c.write(websocket.CloseMessage, []byte{})
				return
			}

			c.underline.SetWriteDeadline(time.Now().Add(c.server.config.WriteTimeout))
			res, err := c.underline.NextWriter(c.messageType)
			if err != nil {
				return
			}
			res.Write(msg)

			n := len(c.send)
			for i := 0; i < n; i++ {
				res.Write(<-c.send)
			}

			if err := res.Close(); err != nil {
				return
			}

		case <-ticker.C:
			if err := c.write(websocket.PingMessage, []byte{}); err != nil {
				return
			}
		}
	}
}

func (c *Connection) reader() {
	defer func() {
		c.Disconnect()
	}()
	conn := c.underline

	conn.SetReadLimit(c.server.config.MaxMessageSize)
	conn.SetReadDeadline(time.Now().Add(c.server.config.PongTimeout))
	conn.SetPongHandler(func(s string) error {
		conn.SetReadDeadline(time.Now().Add(c.server.config.PongTimeout))
		return nil
	})

	for {
		if _, data, err := conn.ReadMessage(); err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway) {
				c.EmitError(err.Error())
			}
			break
		} else {
			c.messageReceived(data)
		}

	}
}

// messageReceived checks the incoming message and fire the nativeMessage listeners or the event listeners (ws custom message)
func (c *Connection) messageReceived(data []byte) {

	if bytes.HasPrefix(data, websocketMessagePrefixBytes) {
		customData := string(data)
		//it's a custom ws message
		receivedEvt := getWebsocketCustomEvent(customData)
		listeners := c.onEventListeners[receivedEvt]
		if listeners == nil { // if not listeners for this event exit from here
			return
		}
		customMessage, err := websocketMessageDeserialize(receivedEvt, customData)
		if customMessage == nil || err != nil {
			return
		}

		for i := range listeners {
			if fn, ok := listeners[i].(func()); ok { // its a simple func(){} callback
				fn()
			} else if fnString, ok := listeners[i].(func(string)); ok {

				if msgString, is := customMessage.(string); is {
					fnString(msgString)
				} else if msgInt, is := customMessage.(int); is {
					// here if server side waiting for string but client side sent an int, just convert this int to a string
					fnString(strconv.Itoa(msgInt))
				}

			} else if fnInt, ok := listeners[i].(func(int)); ok {
				fnInt(customMessage.(int))
			} else if fnBool, ok := listeners[i].(func(bool)); ok {
				fnBool(customMessage.(bool))
			} else if fnBytes, ok := listeners[i].(func([]byte)); ok {
				fnBytes(customMessage.([]byte))
			} else {
				listeners[i].(func(interface{}))(customMessage)
			}

		}
	} else {
		// it's native websocket message
		for i := range c.onNativeMessageListeners {
			c.onNativeMessageListeners[i](data)
		}
	}

}

func (c *Connection) ID() string {
	return c.id
}

func (c *Connection) fireDisconnect() {
	for i := range c.onDisconnectListeners {
		c.onDisconnectListeners[i]()
	}
}

func (c *Connection) To(to string) Emmiter {

	return newEmmiter(c.namespace, to)
}

func (c *Connection) OnDisconnect(cb DisconnectFunc) {
	c.onDisconnectListeners = append(c.onDisconnectListeners, cb)
}

func (c *Connection) OnError(cb ErrorFunc) {
	c.onErrorListeners = append(c.onErrorListeners, cb)
}

func (c *Connection) EmitError(errorMessage string) {
	for _, cb := range c.onErrorListeners {
		cb(errorMessage)
	}
}

func (c *Connection) EmitMessage(nativeMessage []byte) error {
	mp := websocketMessagePayload{c.namespace.name, c.id, nativeMessage}
	c.server.messages <- mp
	return nil
}

func (c *Connection) Emit(event string, data interface{}) error {
	message, err := websocketMessageSerialize(event, data)
	if err != nil {
		return err
	}
	c.EmitMessage([]byte(message))
	return nil
}

func (c *Connection) OnMessage(cb NativeMessageFunc) {
	c.onNativeMessageListeners = append(c.onNativeMessageListeners, cb)
}

func (c *Connection) On(event string, cb MessageFunc) {
	if c.onEventListeners[event] == nil {
		c.onEventListeners[event] = make([]MessageFunc, 0)
	}

	c.onEventListeners[event] = append(c.onEventListeners[event], cb)
}

func (c *Connection) List(room string) []string {

	if c.namespace == nil {

		return make([]string, 0)
	}

	return c.namespace.List(room)
}

func (c *Connection) Join(roomName string) {
	//payload := websocketRoomPayload{c.namespace.name, roomName, c.id}
	//c.server.join <- payload
	// use the new join

	c.namespace.joinRoom(roomName, c.id)

}

func (c *Connection) Leave(roomName string) {
	//payload := websocketRoomPayload{c.namespace.name, roomName, c.id}
	//c.server.leave <- payload
	// use the new leave

	c.namespace.leaveRoom(roomName, c.id)

}

func (c *Connection) Disconnect() error {
	c.server.free <- c // leaves from all rooms, fires the disconnect listeners and finally remove from conn list
	return c.underline.Close()
}

func (c *Connection) Request() *http.Request {
	return c.request
}

func (c *Connection) Get(key string) string {
	return c.data[key]
}

func (c *Connection) Set(key string, value string) {
	c.data[key] = value
}
