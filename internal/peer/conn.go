package peer

import "net"

type Connection struct{
	Conn net.Conn
	Choked bool
	Bitfield []byte
	Peer Peer
}
