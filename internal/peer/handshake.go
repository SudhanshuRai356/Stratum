package peer

import (
	"bytes"
	"errors"
	"net"
	"strconv"
	"time"
)

const (
	protocolString = "BitTorrent protocol"
	handshakeLen = 68
)

func PerformHandshake(p Peer, infoHash [20]byte, peerID [20]byte)(net.Conn,error){
	address := net.JoinHostPort(p.IP, strconv.Itoa(int(p.Port)))
	conn,err:=net.DialTimeout("tcp",address,5*time.Second)
	if err!=nil{
		return nil,err
	}
	if err:= sendHandshake(conn,infoHash,peerID); err!=nil{
		return nil,err
	}
	if err:= receiveHandshake(conn,infoHash);err!=nil{
		return nil,err
	}
	return conn,nil
}

func sendHandshake(conn net.Conn, infoHash [20]byte, peerID [20]byte)error{
	buf := bytes.NewBuffer(nil)
	buf.WriteByte(byte(len(protocolString))) //pstrlen
	buf.WriteString(protocolString) //pstr
	buf.Write(make([]byte,8)) //reserved bytes
	buf.Write(infoHash[:]) //infohash
	buf.Write(peerID[:]) //peerID
	_,err:=conn.Write(buf.Bytes())
	return err
}

func receiveHandshake(conn net.Conn, expected_infoHash [20]byte)error{
	conn.SetReadDeadline(time.Now().Add(5*time.Second))
	resp := make([]byte, handshakeLen)
	_,err:= conn.Read(resp)
	if err != nil{
		return err
	}
	if resp[0]!=byte(len(protocolString)){
		return errors.New("invalid protocol string length")
	}
	if string(resp[1:20]) != protocolString {
		return errors.New("invalid protocol string")
	}
	//extracting infohash
	peerInfoHash := resp [28:48]
	if !bytes.Equal(peerInfoHash, expected_infoHash[:]){
		return errors.New("info hash mismatch")
	}
	return nil
}