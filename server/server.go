package websocket

import (
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
)

// -------------------------------------------------------------------------------------
// -------------------------------------------------------------------------------------
// --------------------------------Server implementation--------------------------------
// -------------------------------------------------------------------------------------
// -------------------------------------------------------------------------------------

type (
	// ConnectionFunc is the callback which fires when a client/connection is connected to the server.
	// Receives one parameter which is the Connection
	ConnectionFunc func(*Connection)
	// Rooms is just a map with key a string and  value slice of string
	Rooms map[string][]string

	// websocketRoomPayload is used as payload from the connection to the server
	websocketRoomPayload struct {
		namespace    string
		roomName     string
		connectionID string
	}

	// payloads, connection -> server
	websocketMessagePayload struct {
		namespace string
		to        string
		data      []byte
	}

	Server struct {
		config                Config
		put                   chan *Connection
		free                  chan *Connection
		connections           map[string]*Connection
		coLock                sync.RWMutex
		messages              chan websocketMessagePayload
		onConnectionListeners []ConnectionFunc
		broadcast Emmiter

		// NameSpace
		namespaces map[string]*NameSpace
		nsLock	    sync.Mutex
	}
)

// server implementation

// New creates a websocket server and returns it
func New(cfg ...Config) *Server {
	c := Config{}
	if len(cfg) > 1 {
		c = cfg[0]
	}
	c = c.validate()
	return newServer(c)
}

// newServer creates a websocket server and returns it
func newServer(c Config) *Server {

	s := &Server{
		config:                c,
		put:                   make(chan *Connection),
		free:                  make(chan *Connection),
		connections:           make(map[string]*Connection),
		messages:              make(chan websocketMessagePayload, 4096), // buffered because messages can be sent/received immediately on connection connected
		onConnectionListeners: make([]ConnectionFunc,0),
		namespaces:            make(map[string]*NameSpace),
	}

	// default  namespace
    defaultNameSpace := &NameSpace{s,defaultNameSpaceName,make(Rooms),sync.Mutex{}}
	s.namespaces[defaultNameSpaceName] = defaultNameSpace
	s.broadcast = newEmmiter(defaultNameSpace, All)

	return s
}

func (s *Server) Handler() http.Handler {
	// build the upgrader once
	c := s.config
	upgrader := websocket.Upgrader{ReadBufferSize: c.ReadBufferSize, WriteBufferSize: c.WriteBufferSize, Error: c.Error, CheckOrigin: c.CheckOrigin}
	return http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		// Upgrade upgrades the HTTP server connection to the WebSocket protocol.
		//
		// The responseHeader is included in the response to the client's upgrade
		// request. Use the responseHeader to specify cookies (Set-Cookie) and the
		// application negotiated subprotocol (Sec--Protocol).
		//
		// If the upgrade fails, then Upgrade replies to the client with an HTTP error
		// response.
		conn, err := upgrader.Upgrade(res, req, res.Header())
		if err != nil {
			http.Error(res, "Websocket Error: "+err.Error(), http.StatusServiceUnavailable)
			return
		}
		s.handleConnection(conn, req)
	})
}

func (s *Server) handleConnection(websocketConn *websocket.Conn, req *http.Request) {
	c := newConnection(websocketConn, s, req)
	s.put <- c
	go c.writer()
	c.reader()
}


// OnConnection this is the main event you, as developer, will work with each of the websocket connections
func (s *Server) OnConnection(cb ConnectionFunc) {
	s.onConnectionListeners = append(s.onConnectionListeners, cb)
}


func (s *Server) onPut(c *Connection) {


	s.coLock.Lock()
    s.connections[c.id] = c
	s.coLock.Unlock()

    c.namespace.joinRoom(c.id,c.id)   // join a default room
    
	for i := range s.onConnectionListeners {
		go s.onConnectionListeners[i](c)
	}


}

func (s *Server) onFree(c *Connection) {

	//  todo checks  namespace

	if _, found := s.connections[c.id]; found {

		

		for roomName := range c.namespace.rooms {
            c.namespace.leaveRoom(roomName,c.id)
		}
		s.coLock.Lock()
		delete(s.connections, c.id)
		close(c.send)
        s.coLock.Unlock()
		
        
        c.fireDisconnect()
	}

}

// Serve starts the websocket server
func (s *Server) Serve() {
	go s.serve()
}

//  to the default namespace
func (s *Server) To(to string) Emmiter {

	// send to  default namespace's room
	namespance := s.namespaces[defaultNameSpaceName]

	return newEmmiter(namespance, to)
}

func (s *Server) ToAll() Emmiter {

	return s.broadcast
}

func (s *Server) List(room string) []*Connection {

	namespance := s.namespaces[defaultNameSpaceName]

	var connList []*Connection
	for _, connectionIDInsideRoom := range namespance.rooms[room] {
		if c, connected := s.connections[connectionIDInsideRoom]; connected {
			connList = append(connList, c)
		}
	}
	return connList
}

func (s *Server) GetConnection(cid string) *Connection {

	conn, ok := s.connections[cid]
	if !ok {
		return nil
	}
	return conn
}

func (s *Server) Of(namespaceName string) *NameSpace {

	if namespace, ok := s.namespaces[namespaceName]; ok {
		return namespace
	}

	// if not  we just create a,  but not save to server
	namespace := &NameSpace{server: s, name: namespaceName}
	return namespace
}

func (s *Server) serve() {
	for {
		select {
		case c := <-s.put: // connection established
			s.onPut(c)
		case c := <-s.free: // connection closed
			s.onFree(c)
		case msg := <-s.messages: // message received from the connection
			if msg.to == All {
				for connID, c := range s.connections {
					select {
					case s.connections[connID].send <- msg.data: //send the message back to the connection in order to send it to the client
					default:
						close(c.send)
						delete(s.connections, connID)
						c.fireDisconnect()

					}

				}

			} else if namespace, ok := s.namespaces[msg.namespace]; ok {

				// get namespace first
				if _, ok := namespace.rooms[msg.to]; ok {

					for _, connectionIDInsideRoom := range namespace.rooms[msg.to] {
						if c, connected := s.connections[connectionIDInsideRoom]; connected {
							c.send <- msg.data //here we send it without need to continue below
						} else {
							// the connection is not connected but it's inside the room, we remove it on disconnect but for ANY CASE:
                            c.namespace.leaveRoom(msg.to, c.id)
						}
					}

				}
				// it suppose to send the message to a room

			}

		}

	}
}
