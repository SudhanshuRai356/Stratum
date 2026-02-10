package dht

import (
	"encoding/hex"
	"errors"
	"net"
)
type Node struct {
	ID   string `json:"id"`   // node ID hex string format
	IP   string `json:"ip"`   // IP address string for json
	Port int    `json:"port"` // Port number for json
}
func (n *Node) Validate() error { //validate node's data
	// Node ID must be 20 bytes that is 40 hex chars
	if len(n.ID) != 40 {
		return errors.New("node ID must be 40 hex characters")
	}

	// ID must be hexadecimal
	_, err := hex.DecodeString(n.ID)
	if err != nil {
		return errors.New("node ID must be valid hex")
	}

	ip := net.ParseIP(n.IP)
	if ip == nil {
		return errors.New("invalid IP address")
	}

	// Port in usable range
	if n.Port < 1 || n.Port > 65535 {
		return errors.New("port must be between 1 and 65535")
	}

	return nil
}