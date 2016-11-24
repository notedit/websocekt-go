package websocket

import (
    "sync"

)

const (
	defaultNameSpaceName = ""
	nameSpaceFormKey     = "namespace"
)


type NameSpace struct {
	server *Server
	name   string
	rooms  Rooms      // by default a connection is joined to a room which has the connection id as its name
	mu     sync.Mutex // for rooms
}

func (n *NameSpace) List(room string) []string {

    n.mu.Lock()
    defer n.mu.Unlock()

    connectionIDs,_ := n.rooms[room]

    return connectionIDs

}

func (n *NameSpace) To(to string) Emmiter {
	// send to  default namespace's room
	return newEmmiter(n, to)

}

func (n *NameSpace) joinRoom(roomName string, connID string) {

    n.mu.Lock()
    defer n.mu.Unlock()

    if _,ok := n.rooms[roomName]; !ok {
        n.rooms[roomName] = make([]string,0)
    }

    n.rooms[roomName] = append(n.rooms[roomName], connID)

}


func (n *NameSpace) leaveRoom(roomName string, connID string) {


    n.mu.Lock()
    defer n.mu.Unlock()

    if room,ok := n.rooms[roomName]; ok {
        for i := range room {
            if room[i] == connID {
                n.rooms[roomName][i]  = n.rooms[roomName][len(n.rooms[roomName])-1]
                n.rooms[roomName] = n.rooms[roomName][:len(n.rooms[roomName])-1]
                break
            }    
        }

        if len(n.rooms[roomName]) == 0 {
            delete(n.rooms,roomName)
        }
        
    }

}
