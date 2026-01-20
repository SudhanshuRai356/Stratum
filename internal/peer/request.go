package peer

import (
	"encoding/binary"
	"net"
)

const Blocksize = 16 * 1024 //16KB

func SendReq(conn net.Conn, pieceIndex, begin, length uint32) error {
	buf := make([]byte, 17)
	//length prefix 13
	binary.BigEndian.PutUint32(buf[0:4], 13)
	// msg ID 6
	buf[4] = 6
	// payload
	binary.BigEndian.PutUint32(buf[5:9], pieceIndex)
	binary.BigEndian.PutUint32(buf[9:13], begin)
	binary.BigEndian.PutUint32(buf[13:17], length)
	_,err:=conn.Write(buf)
	return err
}
