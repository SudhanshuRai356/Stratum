package peer

import (
	"encoding/binary"
	"io"
)

type Message struct{
	ID byte
	Payload []byte
}

func ReadMessage(r io.Reader)(*Message, error){ //reads a single bittorent message from the peer
	lenBuf:=make([]byte,4)
	if _,err:=io.ReadFull(r,lenBuf); err!=nil{
		return nil,err
	}
	length := binary.BigEndian.Uint32(lenBuf)

	//keep-alive
	if length==0{
		return &Message{ID: 255},nil
	}

	msgBuf:= make([]byte,length)
	if _,err:=io.ReadFull(r,msgBuf);err!=nil{
		return nil,err
	}
	return &Message{
		ID: msgBuf[0],
		Payload: msgBuf[1:],
	},nil

}