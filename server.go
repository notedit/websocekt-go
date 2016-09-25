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
		roomName     string
		connectionID string
	}

	// payloads, connection -> server
	websocketMessagePayload struct {
		to   string
		data []byte
	}

	Server struct {
		config                Config
		put                   chan *connection
		free                  chan *connection
		connections           map[string]*connection
		join                  chan websocketRoomPayload
		leave                 chan websocketRoomPayload
		rooms                 Rooms      // by default a connection is joined to a room which has the connection id as its name
		mu                    sync.Mutex // for rooms
		messages              chan websocketMessagePayload
		onConnectionListeners []ConnectionFunc
		//connectionPool        *sync.Pool // sadly I can't make this because the websocket connection is live until is closed.
		broadcast Emmiter
	}
)

var _ Server = &Server{}

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
	}

	s.broadcast = newEmmiter(s, All)

	// go s.serve() // start the ws server
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
        if c.BeforeConnectionFunc != nil {
            err,code := c.BeforeConnectionFunc(req)
            if err != nil {
                http.Error(res, err.Error(),code)
                return
            }
        }

		s.handleConnection(conn, req)
	})
}

func (s *Server) handleConnection(websocketConn UnderlineConnection, req *http.Request) {
	c := newConnection(websocketConn, s, req)
	s.put <- c
	go c.writer()
	c.reader()
}

// OnConnection this is the main event you, as developer, will work with each of the websocket connections
func (s *Server) OnConnection(cb ConnectionFunc) {
	s.onConnectionListeners = append(s.onConnectionListeners, cb)
}

func (s *Server) joinRoom(roomName string, connID string) {
	s.mu.Lock()
	if s.rooms[roomName] == nil {
		s.rooms[roomName] = make([]string, 0)
	}
	s.rooms[roomName] = append(s.rooms[roomName], connID)
	s.mu.Unlock()
}

func (s *Server) leaveRoom(roomName string, connID string) {
	s.mu.Lock()
	if s.rooms[roomName] != nil {
		for i := range s.rooms[roomName] {
			if s.rooms[roomName][i] == connID {
				s.rooms[roomName][i] = s.rooms[roomName][len(s.rooms[roomName])-1]
				s.rooms[roomName] = s.rooms[roomName][:len(s.rooms[roomName])-1]
				break
			}
		}
		if len(s.rooms[roomName]) == 0 { // if room is empty then delete it
			delete(s.rooms, roomName)
		}
	}

	s.mu.Unlock()
}

// Serve starts the websocket server
func (s *Server) Serve() {
	go s.serve()
}

func (s *Server) To(to string) Emmiter {

	if to == All { //send to all
		return s.broadcast
	}

	// send to a room
	return newEmmiter(s, to)
}

func (s *Server) GetConnection(cid string) Connection {

	conn, ok := s.connections[cid]
	if !ok {
		return nil
	}
	return conn
}

func (s *Server) List(room string) []Connection {

	var connList []Connection
	for _, connectionIDInsideRoom := range s.rooms[room] {
		if c, connected := s.connections[connectionIDInsideRoom]; connected {
			connList.append(c)
		}
	}
	return connList
}

func (s *Server) Amount() int {

	return len(s.connections)
}

func (s *Server) serve() {
	for {
		select {
		case c := <-s.put: // connection established
			s.connections[c.id] = c
			// make and join a room with the connection's id
			s.rooms[c.id] = make([]string, 0)
			s.rooms[c.id] = []string{c.id}
			for i := range s.onConnectionListeners {
				s.onConnectionListeners[i](c)
			}
		case c := <-s.free: // connection closed
			if _, found := s.connections[c.id]; found {
				// leave from all rooms
				for roomName := range s.rooms {
					s.leaveRoom(roomName, c.id)
				}
				delete(s.connections, c.id)
				close(c.send)
				c.fireDisconnect()

			}
		case join := <-s.join:
			s.joinRoom(join.roomName, join.connectionID)
		case leave := <-s.leave:
			s.leaveRoom(leave.roomName, leave.connectionID)
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

			} else if _, ok := s.rooms[msg.to]; ok {
				// it suppose to send the message to a room
				for _, connectionIDInsideRoom := range s.rooms[msg.to] {
					if c, connected := s.connections[connectionIDInsideRoom]; connected {
						c.send <- msg.data //here we send it without need to continue below
					} else {
						// the connection is not connected but it's inside the room, we remove it on disconnect but for ANY CASE:
						s.leaveRoom(c.id, msg.to)
					}
				}
			}

		}

	}
}
