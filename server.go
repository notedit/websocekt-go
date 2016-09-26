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
	ConnectionFunc func(Connection)
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
		join                  chan websocketRoomPayload
		leave                 chan websocketRoomPayload
		mu                    sync.Mutex // for namespaces
		messages              chan websocketMessagePayload
		onConnectionListeners []ConnectionFunc
		//connectionPool        *sync.Pool // sadly I can't make this because the websocket connection is live until is closed.
		broadcast Emmiter

		// NameSpace
		namespaces map[string]*NameSpace
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
		put:                   make(chan *connection),
		free:                  make(chan *connection),
		connections:           make(map[string]*connection),
		join:                  make(chan websocketRoomPayload, 1), // buffered because join can be called immediately on connection connected
		leave:                 make(chan websocketRoomPayload),
		rooms:                 make(Rooms),
		messages:              make(chan websocketMessagePayload, 1), // buffered because messages can be sent/received immediately on connection connected
		onConnectionListeners: make([]ConnectionFunc, 0),
		namespaces:            make(map[string]*NameSpace),
	}

	s.broadcast = newEmmiter(s, All)

	// default  namespace
	s.namespaces[defaultNameSpaceName] = &NameSpace{name: defaultNameSpaceName, server: s, rooms: make(Rooms)}

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

func (s *Server) handleConnection(websocketConn *websocekt.Conn, req *http.Request) {
	c := newConnection(websocketConn, s, req)
	s.put <- c
	go c.writer()
	c.reader()
}

// OnConnection this is the main event you, as developer, will work with each of the websocket connections
func (s *Server) OnConnection(cb ConnectionFunc) {
	s.onConnectionListeners = append(s.onConnectionListeners, cb)
}

func (s *Server) joinRoom(namespaceName string, roomName string, connID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	namespace := s.namespaces[namespaceName]
	if namespace == nil {
		return
	}
	if namespace.rooms[roomName] == nil {
		namespace.rooms[roomName] = make([]string, 0)
	}
	namespace.rooms[roomName] = append(namespace.rooms[roomName], connID)
}

func (s *Server) leaveRoom(namespaceName string, roomName string, connID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	namespace := s.namespaces[namespaceName]
	if namespace == nil {
		return
	}

	if namespace.rooms[roomName] != nil {
		for i := range namespace.rooms[roomName] {
			if namespace.rooms[roomName][i] == connID {
				namespace.rooms[roomName][i] = namespace.rooms[roomName][len(s.rooms[roomName])-1]
				namespace.rooms[roomName] = namespace.rooms[roomName][:len(namespace.rooms[roomName])-1]
				break
			}
		}
		if len(namespace.rooms[roomName]) == 0 { // if room is empty then delete it
			delete(namespace.rooms, roomName)
		}
	}

	// todo  if namespce is empty  we need to delete it
}

func (s *Server) onPut(c *Connection) {

	s.connections[c.id] = c
	// make and join a room with the connection's id
	s.rooms[c.id] = make([]string, 0)
	s.rooms[c.id] = []string{c.id}

	namespaceName := c.Request().FormValue(nameSpaceFormKey)

	if s.namespaces[namespaceName] == nil {

		s.mu.Lock()
		defer s.mu.Unlock()
		namespce := &NameSpace{server: s, name: namespaceName, rooms: make(Rooms)}
		namespce.rooms[c.id] = make([]string, 0)
		namespce.rooms[c.id] = []string{c.id}
		s.namespaces[namespaceName] = namespce
		c.namespace = namespce

	}

	for i := range s.onConnectionListeners {
		s.onConnectionListeners[i](c)
	}

}

func (s *Server) onFree(c *Connection) {

	//  todo checks  namespace

	if _, found := s.connections[c.id]; found {

		s.mu.Lock()
		defer s.mu.Unlock()

		for roomName := range c.namespace.rooms {
			s.leaveRoom(c.namespace.name, roomName, c.id)
		}
		delete(s.connections, c.id)
		close(c.send)
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
			connList.append(c)
		}
	}
	return connList
}

func (s *Server) GetConnection(cid string) Connection {

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
		case join := <-s.join:
			s.joinRoom(join.namespace, join.roomName, join.connectionID)
		case leave := <-s.leave:
			s.leaveRoom(leave.namespace, leave.roomName, leave.connectionID)
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
							s.leaveRoom(msg.namespace, c.id, msg.to)
						}
					}

				}
				// it suppose to send the message to a room

			}

		}

	}
}
