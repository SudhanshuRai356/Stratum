package tracker

import (
	"fmt"
	"math/rand"
	"net"
	"net/url"
	"time"
	"encoding/binary"

	"github.com/Kushal-39/Stratum/internal/torrent"
)

func AnnounceUDP(announce string, t *torrent.Torrent) ([]Peer, error) {
	u, err := url.Parse(announce)
	if err != nil {
		return nil, err
	}

	addr := net.JoinHostPort(u.Hostname(), u.Port()) //parsing udp

	conn, err := net.DialTimeout("udp", addr, 5*time.Second)
	if err != nil {
		return nil, err
	}

	transactionID := rand.Uint32()
	buf := make([]byte, 16)

	// protocol ID: 0x41727101980
	binary.BigEndian.PutUint64(buf[0:8], 0x41727101980)
	binary.BigEndian.PutUint32(buf[8:12], 0) // action = connect
	binary.BigEndian.PutUint32(buf[12:16], transactionID)

	resp:=make([]byte,16)
	_,err=conn.Read(resp)
	if err!=nil{
		return nil,err
	}

	action:= binary.BigEndian.Uint32(resp[0:4])
	respTxn:= binary.BigEndian.Uint32(resp[4:8])
	connectionID:=binary.BigEndian.Uint32(resp[8:16])

	if action!=0 || respTxn!=transactionID{
		return nil,fmt.Errorf("invalid connect response")
	}

	transactionID = rand.Uint32()
	req:=make([]byte,98)  //udp announce req is 98 bytes long

	binary.BigEndian.PutUint32(req[0:8],connectionID)
	binary.BigEndian.PutUint32(req[8:12],1) // announce
	binary.BigEndian.PutUint32(req[12:16],transactionID)

	copy(req[16:36],t.InfoHash[:])
	copy(req[36:56],[]byte("-ST0001-123456789012"))

	//download
	binary.BigEndian.PutUint64(req[56:64],0)
	//left
	binary.BigEndian.PutUint64(req[64:72],uint64(t.Info.Length))
	//uploaded
	binary.BigEndian.PutUint64(req[72:80],0)

	binary.BigEndian.PutUint32(req[80:84],0) //event

	binary.BigEndian.PutUint32(req[84:88], 0) //IP

	binary.BigEndian.PutUint32(req[88:92],rand.Uint32())

	binary.BigEndian.PutUint32(req[92:96], 0xffffffff)

	binary.BigEndian.PutUint32(req[96:98],6881)//port

	_,err=conn.Write(req)
	if err!=nil{
		return nil,err
	}
	resp = make([]byte, 1500)
	n,err:=conn.Read(resp)
	if err!=nil{
		return nil,err
	}

	action =binary.BigEndian.Uint32(resp[0:4])
	respTxn= binary.BigEndian.Uint32(resp[4:8])

	if action != 1 || respTxn != transactionID{
		return nil,fmt.Errorf("invalid announce response")
	}

	var peers []Peer
	for i:=20;i+6<=n;i+=6{
		ip:=net.IP(resp[i:i+4]).String()
		port:= binary.BigEndian.Uint16(resp[i+4:i+6])
		peers=append(peers, Peer{
			IP: ip,
			Port: port,
		})
	}
	return peers,nil
}
