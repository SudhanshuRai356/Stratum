package dht

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net"
	"sync"
	"time"

	bencode "github.com/jackpal/bencode-go"
)

// Client represents a DHT client
type Client struct {
	nodeID       []byte
	port         int
	conn         *net.UDPConn
	routingTable *RoutingTable
	transactions map[string]chan map[string]interface{}
	txMu         sync.RWMutex
	running      bool
	mu           sync.RWMutex
}

// NewClient creates a new DHT client
func NewClient(port int) (*Client, error) {
	nodeID := GenerateNodeID()

	addr := &net.UDPAddr{
		IP:   net.IPv4zero,
		Port: port,
	}

	conn, err := net.ListenUDP("udp4", addr)
	if err != nil {
		return nil, fmt.Errorf("failed to listen on UDP port %d: %w", port, err)
	}

	client := &Client{
		nodeID:       nodeID,
		port:         port,
		conn:         conn,
		routingTable: NewRoutingTable(nodeID),
		transactions: make(map[string]chan map[string]interface{}),
		running:      false,
	}

	return client, nil
}

// Start begins the DHT client event loop
func (c *Client) Start() error {
	c.mu.Lock()
	if c.running {
		c.mu.Unlock()
		return fmt.Errorf("client already running")
	}
	c.running = true
	c.mu.Unlock()

	go c.readLoop()
	return nil
}

// Stop stops the DHT client
func (c *Client) Stop() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.running {
		return
	}

	c.running = false
	if c.conn != nil {
		c.conn.Close()
	}
}

// Bootstrap initializes the routing table with known nodes
func (c *Client) Bootstrap(bootstrapNodes []Node) error {
	if len(bootstrapNodes) == 0 {
		// Use default bootstrap nodes
		bootstrapNodes = GetDefaultBootstrapNodes()
	}

	fmt.Printf("Bootstrapping DHT with %d nodes...\n", len(bootstrapNodes))

	// Ping nodes concurrently
	type result struct {
		node    Node
		success bool
	}
	resultChan := make(chan result, len(bootstrapNodes))

	for _, node := range bootstrapNodes {
		go func(n Node) {
			if err := n.Validate(); err != nil {
				resultChan <- result{n, false}
				return
			}

			// Ping the node to verify it's alive
			success := c.Ping(n)
			resultChan <- result{n, success}
		}(node)
	}

	// Collect results
	successCount := 0
	for i := 0; i < len(bootstrapNodes); i++ {
		res := <-resultChan
		if res.success {
			c.routingTable.AddNode(res.node)
			successCount++
		}
	}

	if successCount == 0 {
		return fmt.Errorf("failed to bootstrap: no nodes responded")
	}

	// Find nodes close to our own ID to populate routing table
	go c.findNode(c.nodeID)

	fmt.Printf("Successfully bootstrapped with %d nodes\n", successCount)
	return nil
}

// Ping sends a ping query to a node
func (c *Client) Ping(node Node) bool {
	txID := generateTransactionID()

	query := map[string]interface{}{
		"t": txID,
		"y": "q",
		"q": "ping",
		"a": map[string]interface{}{
			"id": string(c.nodeID),
		},
	}

	respChan := make(chan map[string]interface{}, 1)
	c.txMu.Lock()
	c.transactions[txID] = respChan
	c.txMu.Unlock()

	defer func() {
		c.txMu.Lock()
		delete(c.transactions, txID)
		c.txMu.Unlock()
	}()

	if err := c.sendMessage(node, query); err != nil {
		return false
	}

	select {
	case <-respChan:
		return true
	case <-time.After(5 * time.Second):
		return false
	}
}

// FindNode queries a node for nodes close to target
func (c *Client) FindNode(node Node, targetID []byte) []Node {
	txID := generateTransactionID()

	query := map[string]interface{}{
		"t": txID,
		"y": "q",
		"q": "find_node",
		"a": map[string]interface{}{
			"id":     string(c.nodeID),
			"target": string(targetID),
		},
	}

	respChan := make(chan map[string]interface{}, 1)
	c.txMu.Lock()
	c.transactions[txID] = respChan
	c.txMu.Unlock()

	defer func() {
		c.txMu.Lock()
		delete(c.transactions, txID)
		c.txMu.Unlock()
	}()

	if err := c.sendMessage(node, query); err != nil {
		return nil
	}

	select {
	case resp := <-respChan:
		return c.parseNodesResponse(resp)
	case <-time.After(5 * time.Second):
		return nil
	}
}

// GetPeers queries the DHT for peers sharing an infohash
func (c *Client) GetPeers(infoHash [20]byte) []string {
	fmt.Printf("Querying DHT for peers with infohash %x\n", infoHash)

	var allPeers []string
	var mu sync.Mutex

	// Get closest nodes to the infohash
	closestNodes := c.routingTable.GetClosestNodes(infoHash[:], K_BUCKET_SIZE)

	if len(closestNodes) == 0 {
		fmt.Println("No nodes in routing table for get_peers")
		return allPeers
	}

	var wg sync.WaitGroup
	for _, node := range closestNodes {
		wg.Add(1)
		go func(n Node) {
			defer wg.Done()

			txID := generateTransactionID()
			query := map[string]interface{}{
				"t": txID,
				"y": "q",
				"q": "get_peers",
				"a": map[string]interface{}{
					"id":        string(c.nodeID),
					"info_hash": string(infoHash[:]),
				},
			}

			respChan := make(chan map[string]interface{}, 1)
			c.txMu.Lock()
			c.transactions[txID] = respChan
			c.txMu.Unlock()

			defer func() {
				c.txMu.Lock()
				delete(c.transactions, txID)
				c.txMu.Unlock()
			}()

			if err := c.sendMessage(n, query); err != nil {
				return
			}

			select {
			case resp := <-respChan:
				peers := c.parsePeersResponse(resp)
				mu.Lock()
				allPeers = append(allPeers, peers...)
				mu.Unlock()

				// Also add any nodes returned
				nodes := c.parseNodesResponse(resp)
				for _, node := range nodes {
					c.routingTable.AddNode(node)
				}
			case <-time.After(5 * time.Second):
				return
			}
		}(node)
	}

	wg.Wait()
	fmt.Printf("DHT returned %d peers\n", len(allPeers))
	return allPeers
}

// AnnouncePeer announces that we have a torrent
func (c *Client) AnnouncePeer(infoHash [20]byte, port int) error {
	closestNodes := c.routingTable.GetClosestNodes(infoHash[:], K_BUCKET_SIZE)

	if len(closestNodes) == 0 {
		return fmt.Errorf("no nodes in routing table")
	}

	for _, node := range closestNodes {
		txID := generateTransactionID()

		// First get_peers to get token
		getPeersQuery := map[string]interface{}{
			"t": txID,
			"y": "q",
			"q": "get_peers",
			"a": map[string]interface{}{
				"id":        string(c.nodeID),
				"info_hash": string(infoHash[:]),
			},
		}

		respChan := make(chan map[string]interface{}, 1)
		c.txMu.Lock()
		c.transactions[txID] = respChan
		c.txMu.Unlock()

		if err := c.sendMessage(node, getPeersQuery); err != nil {
			c.txMu.Lock()
			delete(c.transactions, txID)
			c.txMu.Unlock()
			continue
		}

		select {
		case resp := <-respChan:
			c.txMu.Lock()
			delete(c.transactions, txID)
			c.txMu.Unlock()

			// Get token from response
			token := c.getTokenFromResponse(resp)
			if token == "" {
				continue
			}

			// Now announce with token
			announceTxID := generateTransactionID()
			announceQuery := map[string]interface{}{
				"t": announceTxID,
				"y": "q",
				"q": "announce_peer",
				"a": map[string]interface{}{
					"id":        string(c.nodeID),
					"info_hash": string(infoHash[:]),
					"port":      port,
					"token":     token,
				},
			}

			c.sendMessage(node, announceQuery)
		case <-time.After(5 * time.Second):
			c.txMu.Lock()
			delete(c.transactions, txID)
			c.txMu.Unlock()
		}
	}

	return nil
}

// readLoop continuously reads incoming UDP messages
func (c *Client) readLoop() {
	buf := make([]byte, 65536)

	for {
		c.mu.RLock()
		running := c.running
		c.mu.RUnlock()

		if !running {
			break
		}

		c.conn.SetReadDeadline(time.Now().Add(1 * time.Second))
		n, addr, err := c.conn.ReadFromUDP(buf)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue
			}
			continue
		}

		go c.handleMessage(buf[:n], addr)
	}
}

// handleMessage processes an incoming DHT message
func (c *Client) handleMessage(data []byte, addr *net.UDPAddr) {
	msg, err := bencode.Decode(bytes.NewReader(data))
	if err != nil {
		return
	}

	msgDict, ok := msg.(map[string]interface{})
	if !ok {
		return
	}

	msgType, ok := msgDict["y"].(string)
	if !ok {
		return
	}

	switch msgType {
	case "r": // Response
		c.handleResponse(msgDict)
	case "q": // Query
		c.handleQuery(msgDict, addr)
	case "e": // Error
		// Ignore errors for now
	}
}

// handleResponse processes response messages
func (c *Client) handleResponse(msg map[string]interface{}) {
	txID, ok := msg["t"].(string)
	if !ok {
		return
	}

	c.txMu.RLock()
	respChan, exists := c.transactions[txID]
	c.txMu.RUnlock()

	if exists {
		select {
		case respChan <- msg:
		default:
		}
	}

	// Add responding node to routing table
	if r, ok := msg["r"].(map[string]interface{}); ok {
		if idStr, ok := r["id"].(string); ok {
			// We'd need the IP/port from somewhere - skipping for now
			_ = idStr
		}
	}
}

// handleQuery processes query messages
func (c *Client) handleQuery(msg map[string]interface{}, addr *net.UDPAddr) {
	queryType, ok := msg["q"].(string)
	if !ok {
		return
	}

	txID, ok := msg["t"].(string)
	if !ok {
		return
	}

	switch queryType {
	case "ping":
		c.respondPing(txID, addr)
	case "find_node":
		c.respondFindNode(msg, txID, addr)
	case "get_peers":
		c.respondGetPeers(msg, txID, addr)
	case "announce_peer":
		// We don't track peers, so just send success
		c.sendResponse(txID, addr, map[string]interface{}{
			"id": string(c.nodeID),
		})
	}
}

// sendMessage sends a message to a node
func (c *Client) sendMessage(node Node, msg map[string]interface{}) error {
	var buf bytes.Buffer
	err := bencode.Marshal(&buf, msg)
	if err != nil {
		return err
	}
	data := buf.Bytes()

	// Resolve hostname if needed
	ip := net.ParseIP(node.IP)
	if ip == nil {
		// Try to resolve hostname
		addrs, err := net.LookupHost(node.IP)
		if err != nil || len(addrs) == 0 {
			return fmt.Errorf("failed to resolve hostname %s: %w", node.IP, err)
		}
		ip = net.ParseIP(addrs[0])
		if ip == nil {
			return fmt.Errorf("resolved address is not valid IP: %s", addrs[0])
		}
	}

	addr := &net.UDPAddr{
		IP:   ip,
		Port: node.Port,
	}

	_, err = c.conn.WriteToUDP(data, addr)
	return err
}

// sendResponse sends a response message
func (c *Client) sendResponse(txID string, addr *net.UDPAddr, response map[string]interface{}) {
	msg := map[string]interface{}{
		"t": txID,
		"y": "r",
		"r": response,
	}

	var buf bytes.Buffer
	err := bencode.Marshal(&buf, msg)
	if err != nil {
		return
	}
	data := buf.Bytes()

	c.conn.WriteToUDP(data, addr)
}

// respondPing responds to a ping query
func (c *Client) respondPing(txID string, addr *net.UDPAddr) {
	c.sendResponse(txID, addr, map[string]interface{}{
		"id": string(c.nodeID),
	})
}

// respondFindNode responds to a find_node query
func (c *Client) respondFindNode(msg map[string]interface{}, txID string, addr *net.UDPAddr) {
	a, ok := msg["a"].(map[string]interface{})
	if !ok {
		return
	}

	targetStr, ok := a["target"].(string)
	if !ok {
		return
	}

	closestNodes := c.routingTable.GetClosestNodes([]byte(targetStr), K_BUCKET_SIZE)
	nodesCompact := compactNodes(closestNodes)

	c.sendResponse(txID, addr, map[string]interface{}{
		"id":    string(c.nodeID),
		"nodes": nodesCompact,
	})
}

// respondGetPeers responds to a get_peers query
func (c *Client) respondGetPeers(msg map[string]interface{}, txID string, addr *net.UDPAddr) {
	a, ok := msg["a"].(map[string]interface{})
	if !ok {
		return
	}

	infoHashStr, ok := a["info_hash"].(string)
	if !ok {
		return
	}

	// We don't track peers, so return closest nodes
	closestNodes := c.routingTable.GetClosestNodes([]byte(infoHashStr), K_BUCKET_SIZE)
	nodesCompact := compactNodes(closestNodes)

	// Generate a token (simplified - should be based on IP and secret)
	token := generateToken()

	c.sendResponse(txID, addr, map[string]interface{}{
		"id":    string(c.nodeID),
		"token": token,
		"nodes": nodesCompact,
	})
}

// findNode performs iterative find_node to populate routing table
func (c *Client) findNode(targetID []byte) {
	closestNodes := c.routingTable.GetClosestNodes(targetID, ALPHA)

	for _, node := range closestNodes {
		nodes := c.FindNode(node, targetID)
		for _, n := range nodes {
			c.routingTable.AddNode(n)
		}
	}
}

// Helper functions

func generateTransactionID() string {
	b := make([]byte, 2)
	rand.Read(b)
	return string(b)
}

func generateToken() string {
	b := make([]byte, 8)
	rand.Read(b)
	return string(b)
}

func compactNodes(nodes []Node) string {
	compact := make([]byte, 0, len(nodes)*26) // 20 bytes ID + 4 bytes IP + 2 bytes port

	for _, node := range nodes {
		nodeID, err := hex.DecodeString(node.ID)
		if err != nil {
			continue
		}

		ip := net.ParseIP(node.IP)
		if ip == nil {
			continue
		}
		ip = ip.To4()
		if ip == nil {
			continue
		}

		compact = append(compact, nodeID...)
		compact = append(compact, ip...)
		compact = append(compact, byte(node.Port>>8))
		compact = append(compact, byte(node.Port&0xff))
	}

	return string(compact)
}

func (c *Client) parseNodesResponse(resp map[string]interface{}) []Node {
	r, ok := resp["r"].(map[string]interface{})
	if !ok {
		return nil
	}

	nodesStr, ok := r["nodes"].(string)
	if !ok {
		return nil
	}

	return parseCompactNodes([]byte(nodesStr))
}

func parseCompactNodes(compact []byte) []Node {
	nodes := []Node{}

	for i := 0; i+26 <= len(compact); i += 26 {
		nodeID := hex.EncodeToString(compact[i : i+20])
		ip := net.IP(compact[i+20 : i+24]).String()
		port := int(compact[i+24])<<8 | int(compact[i+25])

		nodes = append(nodes, Node{
			ID:   nodeID,
			IP:   ip,
			Port: port,
		})
	}

	return nodes
}

func (c *Client) parsePeersResponse(resp map[string]interface{}) []string {
	r, ok := resp["r"].(map[string]interface{})
	if !ok {
		return nil
	}

	// Check for values (list of compact peer info)
	if values, ok := r["values"].([]interface{}); ok {
		peers := []string{}
		for _, val := range values {
			if peerStr, ok := val.(string); ok {
				peer := parseCompactPeer([]byte(peerStr))
				if peer != "" {
					peers = append(peers, peer)
				}
			}
		}
		return peers
	}

	return nil
}

func parseCompactPeer(compact []byte) string {
	if len(compact) != 6 {
		return ""
	}

	ip := net.IP(compact[0:4]).String()
	port := int(compact[4])<<8 | int(compact[5])

	return fmt.Sprintf("%s:%d", ip, port)
}

func (c *Client) getTokenFromResponse(resp map[string]interface{}) string {
	r, ok := resp["r"].(map[string]interface{})
	if !ok {
		return ""
	}

	token, ok := r["token"].(string)
	if !ok {
		return ""
	}

	return token
}

// GetDefaultBootstrapNodes returns well-known DHT bootstrap nodes
func GetDefaultBootstrapNodes() []Node {
	return []Node{
		{ID: "0000000000000000000000000000000000000001", IP: "router.bittorrent.com", Port: 6881},
		{ID: "0000000000000000000000000000000000000002", IP: "dht.transmissionbt.com", Port: 6881},
		{ID: "0000000000000000000000000000000000000003", IP: "router.utorrent.com", Port: 6881},
		{ID: "0000000000000000000000000000000000000004", IP: "dht.libtorrent.org", Port: 25401},
	}
}

// GetRoutingTable returns the routing table (for saving nodes)
func (c *Client) GetRoutingTable() *RoutingTable {
	return c.routingTable
}
