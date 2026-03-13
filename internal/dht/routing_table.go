package dht

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"math/big"
	"net"
	"sync"
	"time"
)

const (
	K_BUCKET_SIZE = 8  // Maximum nodes per bucket
	ALPHA         = 3  // Parallel lookup parameter
	NODE_ID_LEN   = 20 // 160 bits = 20 bytes
	maxNodesPerIP = 4  // Max nodes allowed from a single IP
)

// RoutingTable manages the DHT routing table with K-buckets
type RoutingTable struct {
	selfID    []byte
	buckets   [NODE_ID_LEN * 8]*KBucket // 160 buckets (one per bit)
	mu        sync.RWMutex
	ipCounts  map[string]int // tracks how many node IDs map to each IP
	ipCountMu sync.Mutex
}

// KBucket stores nodes at a specific distance range
type KBucket struct {
	nodes      []Node
	lastUpdate time.Time
	mu         sync.RWMutex
}

// NewRoutingTable creates a new routing table
func NewRoutingTable(selfID []byte) *RoutingTable {
	rt := &RoutingTable{
		selfID:   selfID,
		ipCounts: make(map[string]int),
	}
	for i := 0; i < NODE_ID_LEN*8; i++ {
		rt.buckets[i] = &KBucket{
			nodes:      []Node{},
			lastUpdate: time.Now(),
		}
	}
	return rt
}

// GenerateNodeID generates a random 20-byte node ID
func GenerateNodeID() []byte {
	id := make([]byte, NODE_ID_LEN)
	rand.Read(id)
	return id
}

// AddNode adds a node to the routing table
func (rt *RoutingTable) AddNode(node Node) error {
	if err := node.Validate(); err != nil {
		return err
	}

	nodeIDBytes, err := hex.DecodeString(node.ID)
	if err != nil {
		return err
	}

	// Reject node IDs that are all zeros or all ones (trivially forged)
	allZero := true
	allOne := true
	for _, b := range nodeIDBytes {
		if b != 0x00 {
			allZero = false
		}
		if b != 0xFF {
			allOne = false
		}
	}
	if allZero || allOne {
		return fmt.Errorf("rejecting trivial node ID")
	}

	// Reject private/loopback IPs (prevents routing table pollution with non-routable addresses)
	ip := net.ParseIP(node.IP)
	if ip != nil && (ip.IsLoopback() || ip.IsUnspecified()) {
		return fmt.Errorf("rejecting non-routable IP: %s", node.IP)
	}

	// Per-IP rate limit: prevent a single IP from sybil-flooding the table
	rt.ipCountMu.Lock()
	if rt.ipCounts[node.IP] >= maxNodesPerIP {
		rt.ipCountMu.Unlock()
		return fmt.Errorf("too many nodes from IP %s", node.IP)
	}
	rt.ipCountMu.Unlock()

	bucketIndex := rt.getBucketIndex(nodeIDBytes)

	rt.mu.Lock()
	bucket := rt.buckets[bucketIndex]
	rt.mu.Unlock()

	bucket.mu.Lock()
	defer bucket.mu.Unlock()

	// Check if node already exists
	for i, existingNode := range bucket.nodes {
		if existingNode.ID == node.ID {
			// Move to end (most recently seen)
			bucket.nodes = append(bucket.nodes[:i], bucket.nodes[i+1:]...)
			bucket.nodes = append(bucket.nodes, node)
			bucket.lastUpdate = time.Now()
			return nil
		}
	}

	// Add new node
	if len(bucket.nodes) < K_BUCKET_SIZE {
		bucket.nodes = append(bucket.nodes, node)
		bucket.lastUpdate = time.Now()
		rt.ipCountMu.Lock()
		rt.ipCounts[node.IP]++
		rt.ipCountMu.Unlock()
	} else {
		// Bucket full - could implement replacement logic here
		// For now, we keep the oldest nodes (they've proven reliable)
	}

	return nil
}

// GetClosestNodes returns K closest nodes to the target ID
func (rt *RoutingTable) GetClosestNodes(targetID []byte, count int) []Node {
	rt.mu.RLock()
	defer rt.mu.RUnlock()

	allNodes := []Node{}
	for _, bucket := range rt.buckets {
		bucket.mu.RLock()
		allNodes = append(allNodes, bucket.nodes...)
		bucket.mu.RUnlock()
	}

	// Sort by XOR distance
	sortedNodes := sortNodesByDistance(targetID, allNodes)

	if len(sortedNodes) > count {
		return sortedNodes[:count]
	}
	return sortedNodes
}

// GetAllNodes returns all nodes in the routing table
func (rt *RoutingTable) GetAllNodes() []Node {
	rt.mu.RLock()
	defer rt.mu.RUnlock()

	allNodes := []Node{}
	for _, bucket := range rt.buckets {
		bucket.mu.RLock()
		allNodes = append(allNodes, bucket.nodes...)
		bucket.mu.RUnlock()
	}
	return allNodes
}

// GetNodeCount returns total number of nodes in routing table
func (rt *RoutingTable) GetNodeCount() int {
	rt.mu.RLock()
	defer rt.mu.RUnlock()

	count := 0
	for _, bucket := range rt.buckets {
		bucket.mu.RLock()
		count += len(bucket.nodes)
		bucket.mu.RUnlock()
	}
	return count
}

// getBucketIndex calculates which bucket a node belongs to based on XOR distance
func (rt *RoutingTable) getBucketIndex(nodeID []byte) int {
	for i := 0; i < len(rt.selfID)*8; i++ {
		// Find the first differing bit
		byteIndex := i / 8
		bitIndex := 7 - (i % 8)

		selfBit := (rt.selfID[byteIndex] >> bitIndex) & 1
		nodeBit := (nodeID[byteIndex] >> bitIndex) & 1

		if selfBit != nodeBit {
			return i
		}
	}
	// If IDs are identical (shouldn't happen), return last bucket
	return len(rt.selfID)*8 - 1
}

// sortNodesByDistance sorts nodes by XOR distance to target
func sortNodesByDistance(targetID []byte, nodes []Node) []Node {
	type nodeDistance struct {
		node     Node
		distance *big.Int
	}

	nodeDistances := make([]nodeDistance, 0, len(nodes))
	targetInt := new(big.Int).SetBytes(targetID)

	for _, node := range nodes {
		nodeIDBytes, err := hex.DecodeString(node.ID)
		if err != nil {
			continue
		}
		nodeInt := new(big.Int).SetBytes(nodeIDBytes)

		// XOR distance
		distance := new(big.Int).Xor(targetInt, nodeInt)
		nodeDistances = append(nodeDistances, nodeDistance{
			node:     node,
			distance: distance,
		})
	}

	// Sort by distance
	for i := 0; i < len(nodeDistances); i++ {
		for j := i + 1; j < len(nodeDistances); j++ {
			if nodeDistances[i].distance.Cmp(nodeDistances[j].distance) > 0 {
				nodeDistances[i], nodeDistances[j] = nodeDistances[j], nodeDistances[i]
			}
		}
	}

	result := make([]Node, len(nodeDistances))
	for i, nd := range nodeDistances {
		result[i] = nd.node
	}
	return result
}

// XORDistance calculates XOR distance between two IDs
func XORDistance(id1, id2 []byte) *big.Int {
	int1 := new(big.Int).SetBytes(id1)
	int2 := new(big.Int).SetBytes(id2)
	return new(big.Int).Xor(int1, int2)
}
