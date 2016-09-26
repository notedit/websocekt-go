package websocket

import "sync"

const (
	defaultNameSpaceName = ""
	nameSpaceFormKey     = "namespace"
)

var defaultNameSpace = &NameSpace{name: defaultNameSpaceName, rooms: make(Rooms)}

type NameSpace struct {
	server *Server
	name   string
	rooms  Rooms      // by default a connection is joined to a room which has the connection id as its name
	mu     sync.Mutex // for rooms
}

func (n *NameSpace) List(room string) []*Connection {

	var connList []*Connection
	for _, connectionIDInsideRoom := range n.rooms[room] {
		if c, connected := n.server.connections[connectionIDInsideRoom]; connected {
			connList.append(c)
		}
	}
	return connList
}

func (n *NameSpace) To(to string) Emmiter {

	// send to  default namespace's room

	return newEmmiter(n, to)

}
