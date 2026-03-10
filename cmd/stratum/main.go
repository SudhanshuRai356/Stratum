package main

import (
	"bytes"
	"container/heap"
	"crypto/sha1"
	"flag"
	"fmt"
	"math/rand"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/Kushal-39/Stratum/internal/dht"
	"github.com/Kushal-39/Stratum/internal/peer"
	"github.com/Kushal-39/Stratum/internal/storage"
	"github.com/Kushal-39/Stratum/internal/torrent"
	"github.com/Kushal-39/Stratum/internal/tracker"
)

func HasPiece(bitfield []byte, pieceIndex uint32) bool {
	if len(bitfield) == 0 {
		return false
	}
	byteIndex := pieceIndex / 8
	bitIndex := pieceIndex % 8
	if byteIndex >= uint32(len(bitfield)) {
		return false
	}
	return (bitfield[byteIndex] & (1 << (7 - bitIndex))) != 0
}

type piece_item struct {
	piece_index        uint32
	availability_count int
}
type piece_priority_queue []*piece_item

func (pq piece_priority_queue) Len() int { return len(pq) }
func (pq piece_priority_queue) Less(i, j int) bool {
	return pq[i].availability_count < pq[j].availability_count // min-heap for rarest first
}
func (pq piece_priority_queue) Swap(i, j int) {
	pq[i], pq[j] = pq[j], pq[i]
}
func (pq *piece_priority_queue) Push(x interface{}) {
	*pq = append(*pq, x.(*piece_item))
}
func (pq *piece_priority_queue) Pop() interface{} {
	old := *pq
	n := len(old)
	item := old[n-1]
	*pq = old[0 : n-1]
	return item
}

type PieceManager struct { //struct to manage the concurrency in peers
	completed         []bool
	in_progress       []bool
	done_chain        chan uint32
	piece_chain       chan uint32 // channel will be used to avoid the race condition of multiple peers send the same piece
	totalPieces       int
	mu                sync.Mutex
	pieceAvailability []int
	completedCount    int
	badPeers          map[string]int
}

func NewPieceManager(totalPieces int, initialCompleted []bool, initialCount int) *PieceManager {
	pm := &PieceManager{
		completed:         initialCompleted, // Use loaded state
		in_progress:       make([]bool, totalPieces),
		piece_chain:       make(chan uint32, totalPieces),
		done_chain:        make(chan uint32, totalPieces),
		totalPieces:       totalPieces,
		pieceAvailability: make([]int, totalPieces),
		completedCount:    initialCount, // Use loaded count
		badPeers:          make(map[string]int),
	}
	return pm
}
func (pm *PieceManager) ReportBadPeer(ip string) bool {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.badPeers[ip]++
	return pm.badPeers[ip] >= 1 // Strict Policy: 1 Strike = Ban
}

func (pm *PieceManager) IsBanned(ip string) bool {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	return pm.badPeers[ip] >= 1
}

func (pm *PieceManager) allDone() bool {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	return pm.completedCount >= pm.totalPieces
}

func (pm *PieceManager) remainingPieces() int {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	return pm.totalPieces - pm.completedCount
}
func downloadPiece(session *peer.PeerSession, pidx uint32, tor *torrent.Torrent, writer *storage.Writer) error {
	temp_Comp := make([]bool, len(tor.Info.Pieces)/20)
	err := session.RunSinglePiece(pidx, temp_Comp, func(pid uint32) *peer.Downloader {
		return peer.NewDownloader(
			tor,
			pid,
			PipelineDepth,
			writer,
		)
	})
	return err
}
func (pm *PieceManager) update_avail(bitfield []byte) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	for pidx := 0; pidx < pm.totalPieces; pidx++ {
		if HasPiece(bitfield, uint32(pidx)) {
			pm.pieceAvailability[pidx]++
		}
	}
}
func (pm *PieceManager) pick_rarest_piece(peer_bitfield []byte) (uint32, bool) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pq := &piece_priority_queue{}
	heap.Init(pq)
	for pidx := 0; pidx < pm.totalPieces; pidx++ {
		if pm.completed[pidx] || pm.in_progress[pidx] {
			continue
		}
		if !HasPiece(peer_bitfield, uint32(pidx)) {
			continue
		}
		item := &piece_item{
			piece_index:        uint32(pidx),
			availability_count: pm.pieceAvailability[pidx],
		}
		heap.Push(pq, item)
	}
	if pq.Len() == 0 {
		return 0, false
	}
	rarest_item := heap.Pop(pq).(*piece_item)
	pm.in_progress[rarest_item.piece_index] = true
	return rarest_item.piece_index, true
}
func (pm *PieceManager) mark_piece_completed(pidx uint32) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	if !pm.completed[pidx] {
		pm.completed[pidx] = true
		pm.completedCount++
	}
	pm.in_progress[pidx] = false
}
func (pm *PieceManager) mark_piece_failed(pidx uint32) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.in_progress[pidx] = false
}
func VerifyPiece(writer *storage.Writer, pieceIndex uint32, tor *torrent.Torrent) bool {
	// 1. Calculate offset and length
	pieceLen := tor.Info.PieceLength
	// Handle last piece (might be smaller)
	if int(pieceIndex) == len(tor.Info.Pieces)/20-1 {
		remaining := int(tor.Info.Length - (int64(pieceIndex) * int64(pieceLen)))
		pieceLen = remaining
	}

	// 2. Read the piece data from disk/storage
	buf := make([]byte, pieceLen)
	offset := int64(pieceIndex) * int64(tor.Info.PieceLength)

	// Note: You might need to add a ReadAt method to your storage.Writer
	// or access the underlying file directly. Assuming writer has a ReadAt:
	_, err := writer.ReadAt(buf, offset)
	if err != nil {
		fmt.Printf("Verification error: failed to read piece %d: %v\n", pieceIndex, err)
		return false
	}

	// 3. Calculate SHA1
	hash := sha1.Sum(buf)

	// 4. Compare with expected hash from metadata
	start := pieceIndex * 20
	end := start + 20
	expected := []byte(tor.Info.Pieces[start:end])

	if !bytes.Equal(hash[:], expected) {
		return false
	}
	return true
}
func worker(pe tracker.Peer, pm *PieceManager, tor *torrent.Torrent, writer *storage.Writer, seeder *peer.Seeder, wg *sync.WaitGroup) {
	defer wg.Done()

	// 1. CHECK REPUTATION BEFORE CONNECTING
	if pm.IsBanned(pe.IP) {
		// fmt.Printf("Skipping banned peer %s\n", pe.IP)
		return
	}

	pid := generatePeerID()
	p := peer.Peer{IP: pe.IP, Port: pe.Port}

	con, reserved, e := peer.PerformHandshake(p, tor.InfoHash, pid)
	if e != nil {
		return
	}
	defer con.Close()

	session := peer.NewPeerSession(con, p, tor.InfoHash, pid, reserved)
	if err := session.WaitForReady(); err != nil {
		return
	}

	pm.update_avail(session.Bitfield)

	for !pm.allDone() {
		// Double check ban status (in case they were banned by another worker)
		if pm.IsBanned(pe.IP) {
			return
		}

		piece_index, available := pm.pick_rarest_piece(session.Bitfield)
		if !available {
			return
		}

		fmt.Printf("Worker %s downloading piece %d\n", p.IP, piece_index)

		e2 := downloadPiece(session, piece_index, tor, writer)
		if e2 != nil {
			fmt.Printf("Error downloading piece %d from %s: %v\n", piece_index, p.IP, e2)
			pm.mark_piece_failed(piece_index)
			return
		}

		// 2. VERIFY INTEGRITY
		// (Assuming you added ReadAt to storage or open the file manually)
		valid := VerifyPiece(writer, piece_index, tor)

		if valid {
			// SUCCESS
			pm.mark_piece_completed(piece_index)
			fmt.Printf("Completed & Verified piece %d (%d/%d)\n", piece_index, pm.completedCount, pm.totalPieces)
			go func() {
				pm.mu.Lock()
				stateCopy := make([]bool, len(pm.completed))
				copy(stateCopy, pm.completed)
				pm.mu.Unlock()

				writer.SaveState(stateCopy)
			}()

			// NEW: Tell the seeder to update its bitfield and shout to the swarm
			if seeder != nil {
				seeder.MarkPieceAvailable(piece_index)
				seeder.BroadcastHave(piece_index)
			}

		} else {
			fmt.Printf("SECURITY ALERT: Peer %s sent corrupt data for piece %d! Banning...\n", p.IP, piece_index)

			pm.ReportBadPeer(p.IP)            // Add strike / Ban
			pm.mark_piece_failed(piece_index) // Reset piece so honest peers can download it
			return                            // Disconnect immediately
		}
	}
}

const PipelineDepth = 5
const ReannounceInterval = 30 * time.Second

func main() {
	rand.Seed(time.Now().UnixNano())

	outDir := flag.String("o", ".", "output directory")
	flag.Parse()

	if flag.NArg() < 1 {
		fmt.Println("Usage: stratum [-o output_dir] <path-to-torrent-file-or-magnet-link>")
		os.Exit(1)
	}

	input := flag.Arg(0)
	var t *torrent.Torrent
	var err error

	// Detect if input is magnet link or torrent file
	if strings.HasPrefix(input, "magnet:") {
		fmt.Println("Detected magnet link, fetching metadata...")
		t, err = handleMagnetLink(input)
		if err != nil {
			panic(fmt.Sprintf("Failed to get metadata from magnet link: %v", err))
		}
	} else {
		// Parse torrent file
		t, err = torrent.ParseTorrent(input)
		if err != nil {
			panic(err)
		}
	}

	fmt.Println("Torrent parsed:", t.Info.Name)

	// NEW: Build the FileInfo slice for the storage engine
	var storageFiles []storage.FileInfo
	var totalLength int64

	if len(t.Info.Files) > 0 {
		fmt.Println("Mode: Multi-file directory")
		for _, f := range t.Info.Files {
			filePath := strings.Join(f.Path, "/")
			fmt.Printf("  - %s (%d bytes)\n", filePath, f.Length)
			totalLength += int64(f.Length)

			storageFiles = append(storageFiles, storage.FileInfo{
				Path:   f.Path,
				Length: int64(f.Length),
			})
		}
	} else {
		fmt.Println("Mode: Single file")
		totalLength = t.Info.Length

		storageFiles = append(storageFiles, storage.FileInfo{
			Path:   nil, 
			Length: t.Info.Length,
		})
	}

	t.Info.Length = totalLength
	fmt.Println("Total Size:", t.Info.Length)
	absOutDir, err := filepath.Abs(*outDir)
	if err != nil {
		panic(err)
	}

	outputPath := filepath.Join(absOutDir, t.Info.Name)
	dht_node_path := filepath.Join(absOutDir, "dht_nodes.json")
	dht_nodes := dht.LoadNodes(dht_node_path)

	// Initialize DHT client
	fmt.Println("Starting DHT client...")
	dhtClient, err := dht.NewClient(6881)
	if err != nil {
		fmt.Printf("Failed to create DHT client: %v\n", err)
		dhtClient = nil
	} else {
		err = dhtClient.Start()
		if err != nil {
			fmt.Printf("Failed to start DHT client: %v\n", err)
			dhtClient = nil
		} else {
			// Bootstrap DHT
			if len(dht_nodes) > 0 {
				fmt.Printf("Bootstrapping DHT with %d saved nodes\n", len(dht_nodes))
				err = dhtClient.Bootstrap(dht_nodes)
			} else {
				fmt.Println("Bootstrapping DHT with default nodes")
				err = dhtClient.Bootstrap(nil)
			}
			if err != nil {
				fmt.Printf("DHT bootstrap failed: %v\n", err)
			} else {
				fmt.Println("DHT bootstrap successful")
				// Give DHT some time to stabilize
				time.Sleep(2 * time.Second)
			}
		}
	}

	//signal to save dht nodes on ctrl c
	sig_chan := make(chan os.Signal, 1)
	signal.Notify(sig_chan, os.Interrupt, syscall.SIGTERM)
	//sub routine to save using the exit
	go func() {
		<-sig_chan //wait for the interrupt signal we are getting from the notify
		fmt.Println("\nReceived interrupt signal, saving DHT nodes and exiting...")
		if dhtClient != nil {
			dhtClient.Stop()
			dht_nodes = dhtClient.GetRoutingTable().GetAllNodes()
		}
		if len(dht_nodes) > 0 {
			err = dht.SaveNodes(dht_node_path, dht_nodes)
			if err != nil {
				fmt.Printf("Failed to save DHT nodes: %v\n", err)
			}
		}
		os.Exit(0)
	}()

	ticker := time.NewTicker(10 * time.Minute) //interval for automatic saving of dht nodes data
	defer ticker.Stop()
	//subroutine to save using the ticker
	go func() {
		for range ticker.C {
			if dhtClient != nil {
				dht_nodes = dhtClient.GetRoutingTable().GetAllNodes()
			}
			if len(dht_nodes) > 0 {
				fmt.Println("Auto-saving DHT nodes...")
				err = dht.SaveNodes(dht_node_path, dht_nodes)
				if err != nil {
					fmt.Printf("Failed to save DHT nodes: %v\n", err)
				}
			}
		}
	}()

	// Step 2: Get peers from tracker and DHT
	peers, err := tracker.Announce(t)
	if err != nil {
		fmt.Printf("Tracker announce failed: %v\n", err)
		peers = []tracker.Peer{}
	}

	fmt.Printf("Tracker returned %d peers\n", len(peers))

	// Also get peers from DHT if available
	if dhtClient != nil {
		fmt.Println("Querying DHT for peers...")
		dhtPeerAddrs := dhtClient.GetPeers(t.InfoHash)
		fmt.Printf("DHT returned %d peer addresses\n", len(dhtPeerAddrs))

		// Convert DHT peer addresses to tracker.Peer format
		// Convert DHT peer addresses to tracker.Peer format
		for _, addr := range dhtPeerAddrs {
			host, portStr, err := net.SplitHostPort(addr)
			if err == nil {
				// You might need to import "strconv" at the top of main.go
				port, err := strconv.Atoi(portStr)
				if err == nil {
					peers = append(peers, tracker.Peer{IP: host, Port: uint16(port)})
				}
			}
		}
		fmt.Printf("Total peers (tracker + DHT): %d\n", len(peers))
	}

	if len(peers) == 0 {
		fmt.Println("Warning: No peers found initially. Will keep trying in download loop...")
	}

	// Step 3: Try to connect to peers and download
	//peerID := generatePeerID()
	totalPieces := int(len(t.Info.Pieces) / 20)
	//completed := make([]bool, totalPieces)

	writer, err := storage.New(outputPath, storageFiles)
	if err != nil {
		panic(err)
	}
	defer writer.Close()

	completedPieces, err := writer.LoadState(totalPieces)
	completedCount := 0

	if err == nil && completedPieces != nil {
		fmt.Println("State file found! Verifying existing data...")
		for i, done := range completedPieces {
			if done {
				// Verify integrity of pieces on disk
				if VerifyPiece(writer, uint32(i), t) {
					completedCount++
				} else {
					completedPieces[i] = false // Corrupt? Mark as missing.
					fmt.Printf("Piece %d corrupt on disk. Queuing for redownload.\n", i)
				}
			}
		}
		fmt.Printf("Resuming download: %d/%d pieces already completed.\n", completedCount, totalPieces)
	} else {
		// Init empty if no state found
		fmt.Println("No state file found. Starting fresh.")
		completedPieces = make([]bool, totalPieces)
	}

	// 3. Pass loaded state to Manager
	pm := NewPieceManager(totalPieces, completedPieces, completedCount)

	// === SEEDING ===
	fmt.Println("\n=== Starting seeder... ===")
	seedID := generatePeerID()
	pm.mu.Lock()
	completedCopy := make([]bool, len(pm.completed))
	copy(completedCopy, pm.completed)
	pm.mu.Unlock()

	seeder, err := peer.NewSeeder(t, writer, completedCopy, seedID, 6881)
	if err != nil {
		fmt.Printf("Failed to start seeder: %v\n", err)
	} else {
		seeder.Start()

		// Announce to DHT that we're seeding
		if dhtClient != nil {
			dhtClient.AnnouncePeer(t.InfoHash, 6881)
			fmt.Println("Announced to DHT as seeder")
		}
	}

	roundNum := 0

	for !pm.allDone() {
		roundNum++
		fmt.Printf("\n=== Round %d: Remaining pieces: %d/%d ===\n", roundNum, pm.remainingPieces(), totalPieces)

		// Always announce at the start of each round to get fresh peers
		fmt.Println("Announcing to tracker...")
		peers, err = tracker.Announce(t)
		if err != nil {
			fmt.Printf("Tracker announce failed: %v\n", err)
			peers = []tracker.Peer{}
		}

		fmt.Printf("Got %d peers from tracker\n", len(peers))

		// Also query DHT for more peers
		if dhtClient != nil {
			fmt.Println("Querying DHT for additional peers...")
			dhtPeerAddrs := dhtClient.GetPeers(t.InfoHash)

			// Convert DHT peer addresses to tracker.Peer format
			// Convert DHT peer addresses to tracker.Peer format
			for _, addr := range dhtPeerAddrs {
				host, portStr, err := net.SplitHostPort(addr)
				if err == nil {
					// You might need to import "strconv" at the top of main.go
					port, err := strconv.Atoi(portStr)
					if err == nil {
						peers = append(peers, tracker.Peer{IP: host, Port: uint16(port)})
					}
				}
			}
			fmt.Printf("DHT added %d more peers\n", len(dhtPeerAddrs))
		}

		if len(peers) == 0 {
			fmt.Println("No peers returned by tracker or DHT. Retrying in 10 seconds...")
			time.Sleep(10 * time.Second)
			continue // Retry the loop, do NOT exit
		}

		fmt.Printf("Total peers this round: %d\n", len(peers))

		// Start workers for all available peers
		var wg sync.WaitGroup
		for _, pe := range peers {
			wg.Add(1)
			go worker(pe, pm, t, writer, seeder, &wg)
		}

		// Wait for all workers to finish this round
		// Worker exit does NOT end the program - we just wait and loop again
		wg.Wait()

		// Log round completion
		fmt.Printf("\nRound %d finished. Completed: %d/%d pieces\n", roundNum, pm.totalPieces-pm.remainingPieces(), pm.totalPieces)

		// Check termination condition - ONLY exit when ALL pieces are done
		if pm.allDone() {
			break
		}

		// Not done - log and continue to next round
		fmt.Println("Round finished, retrying peers...")

		// Brief pause before next round to avoid hammering tracker/peers
		time.Sleep(2 * time.Second)
	}
	fmt.Println("\n=== All pieces downloaded successfully! ===")

	// Save DHT nodes and cleanup
	if dhtClient != nil {
		dht_nodes = dhtClient.GetRoutingTable().GetAllNodes()
		fmt.Printf("Discovered %d DHT nodes during download\n", len(dht_nodes))
	}
	if len(dht_nodes) > 0 {
		err = dht.SaveNodes(dht_node_path, dht_nodes)
		if err != nil {
			fmt.Printf("Failed to save DHT nodes: %v\n", err)
		}
	}

}

// handleMagnetLink processes a magnet link to get torrent metadata
func handleMagnetLink(magnetURI string) (*torrent.Torrent, error) {
	// Parse magnet link
	magnet, err := torrent.Parse_Mag_URL(magnetURI)
	if err != nil {
		return nil, fmt.Errorf("failed to parse magnet link: %w", err)
	}

	fmt.Printf("Parsed magnet link: %s\n", magnet.Name)
	fmt.Printf("InfoHash: %x\n", magnet.Info_hash)
	fmt.Printf("Trackers: %d\n", len(magnet.Trackers))

	// Convert InfoHash to [20]byte
	var infoHash [20]byte
	copy(infoHash[:], magnet.Info_hash)

	// Try to get peers from trackers to fetch metadata
	tempTorrent := &torrent.Torrent{
		InfoHash: infoHash,
	}

	if len(magnet.Trackers) > 0 {
		tempTorrent.Announce = magnet.Trackers[0]
		tempTorrent.AnnounceList = make([][]string, len(magnet.Trackers))
		for i, tr := range magnet.Trackers {
			tempTorrent.AnnounceList[i] = []string{tr}
		}
	}

	peers, err := tracker.Announce(tempTorrent)
	if err != nil {
		return nil, fmt.Errorf("failed to get peers from tracker: %w", err)
	}

	if len(peers) == 0 {
		return nil, fmt.Errorf("no peers available for metadata exchange")
	}

	fmt.Printf("Found %d peers for metadata exchange\n", len(peers))

	// Try to fetch metadata from peers
	for i, pe := range peers {
		fmt.Printf("Attempting metadata exchange with peer %d/%d (%s:%d)\n", i+1, len(peers), pe.IP, pe.Port)

		p := peer.Peer{
			IP:   pe.IP,
			Port: pe.Port,
		}

		peerID := generatePeerID()
		conn, reserved, err := peer.PerformHandshake(p, infoHash, peerID)
		if err != nil {
			continue
		}

		session := peer.NewPeerSession(conn, p, infoHash, peerID, reserved)

		if err := session.WaitForReady(); err != nil {
			conn.Close()
			continue
		}

		if !session.SupportsExtensions {
			fmt.Printf("Peer does not support extensions\n")
			conn.Close()
			continue
		}

		// Fetch metadata
		metadata, err := session.FetchMetadata(infoHash)
		conn.Close() // Close immediately after fetch attempt

		if err != nil {
			fmt.Printf("Failed to fetch metadata: %v\n", err)
			continue
		}

		fmt.Println("Successfully fetched metadata! Verifying...")

		// Reconstruct torrent from metadata
		// CRITICAL CHANGE: If this fails, LOG IT and CONTINUE. Do not return.
		t, err := torrent.ReconstructTorrentFromMetadata(magnet, metadata)
		if err != nil {
			fmt.Printf("Metadata verification failed for peer %s: %v. Trying next peer...\n", pe.IP, err)
			continue
		}

		fmt.Println("Metadata verified and torrent reconstructed successfully!")
		return t, nil
	}

	return nil, fmt.Errorf("failed to fetch valid metadata from any peer")
}

func generatePeerID() [20]byte {
	var id [20]byte
	// Prefix: -ST0001- (Azureus-style)
	copy(id[:], "-ST0001-")

	// Fill the rest with random characters
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	for i := 8; i < 20; i++ {
		id[i] = charset[rand.Intn(len(charset))]
	}
	return id
}
