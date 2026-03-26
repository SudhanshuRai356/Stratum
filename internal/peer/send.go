package peer

import (
	"encoding/binary"
	"net"
)

func SendInterested(conn net.Conn) error { //sends the "interested" message to the peer after receiving unchoked
	buf := make([]byte, 5)
	binary.BigEndian.PutUint32(buf[0:4], 1) //length prefix is 1
	buf[4] = 2                              //msg ID is 2 i.e "interested"
	_, err := conn.Write(buf)
	return err
}
