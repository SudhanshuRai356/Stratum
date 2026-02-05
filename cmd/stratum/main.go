package main

import (
	"container/heap"
	"flag"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

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
}

func NewPieceManager(totalPieces int) *PieceManager {
	pm := &PieceManager{
		completed:         make([]bool, totalPieces),
		in_progress:       make([]bool, totalPieces),
		piece_chain:       make(chan uint32, totalPieces),
		done_chain:        make(chan uint32, totalPieces),
		totalPieces:       totalPieces,
		pieceAvailability: make([]int, totalPieces),
		completedCount:    0,
	}
	return pm
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
func worker(pe tracker.Peer, pm *PieceManager, tor *torrent.Torrent, writer *storage.Writer, wg *sync.WaitGroup) {
	defer wg.Done()

	pid := generatePeerID()
	p := peer.Peer{
		IP:   pe.IP,
		Port: pe.Port,
	}
	// 1. Capture the reserved bytes
	con, reserved, e := peer.PerformHandshake(p, tor.InfoHash, pid)
	if e != nil {
		// Log less aggressively for timeouts, but keep it for errors
		// fmt.Println("Handshake failed with the peer", p.IP, "due to error ", e)
		return
	}
	defer con.Close() // Worker owns the connection lifecycle

	// 2. Pass the reserved bytes to the session
	session := peer.NewPeerSession(con, p, tor.InfoHash, pid, reserved)

	// Wait for bitfield and unchoke - this actually reads messages from the peer
	if err := session.WaitForReady(); err != nil {
		fmt.Printf("Worker %s: Failed to get ready state: %v\n", p.IP, err)
		return
	}

	pm.update_avail(session.Bitfield)

	for !pm.allDone() {
		piece_index, available := pm.pick_rarest_piece(session.Bitfield)
		if !available {
			fmt.Printf("Worker %s: No more pieces available from this peer\n", p.IP)
			return
		}
		fmt.Printf("Worker %s downloading rarest piece %d\n", p.IP, piece_index)

		e2 := downloadPiece(session, piece_index, tor, writer)
		if e2 != nil {
			fmt.Printf("Error downloading piece %d from %s: %v\n", piece_index, p.IP, e2)
			pm.mark_piece_failed(piece_index) // mark for retry
			return                            // Exit worker on error, main loop will retry with new peers
		} else {
			pm.mark_piece_completed(piece_index)
			fmt.Printf("Completed piece %d (%d/%d)\n", piece_index, pm.completedCount, pm.totalPieces)
		}
	}
}

const PipelineDepth = 5
const ReannounceInterval = 30 * time.Second

func allPiecesDone(completed []bool) bool {
	for _, done := range completed {
		if !done {
			return false
		}
	}
	return true
}

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
	fmt.Println("Size:", t.Info.Length)
	absOutDir, err := filepath.Abs(*outDir)
	if err != nil {
		panic(err)
	}

	outputPath := filepath.Join(absOutDir, t.Info.Name)

	// Step 2: Get peers from tracker
	peers, err := tracker.Announce(t)
	if err != nil {
		panic(err)
	}

	if len(peers) == 0 {
		panic("no peers returned by tracker")
	}

	// Step 3: Try to connect to peers and download
	//peerID := generatePeerID()
	totalPieces := int(len(t.Info.Pieces) / 20)
	//completed := make([]bool, totalPieces)

	writer, err := storage.New(outputPath, t.Info.Length) //create a writer to write the downloaded things to the disk
	if err != nil {
		panic(err)
	}
	defer writer.Close()

	pm := NewPieceManager(totalPieces)
	roundNum := 0

	for !pm.allDone() {
		roundNum++
		fmt.Printf("\n=== Round %d: Remaining pieces: %d/%d ===\n", roundNum, pm.remainingPieces(), totalPieces)

		// Always announce at the start of each round to get fresh peers
		fmt.Println("Announcing to tracker...")
		peers, err = tracker.Announce(t)
		if err != nil {
			fmt.Printf("Tracker announce failed: %v. Retrying in 10 seconds...\n", err)
			time.Sleep(10 * time.Second)
			continue // Retry the loop, do NOT exit
		}

		if len(peers) == 0 {
			fmt.Println("No peers returned by tracker. Retrying in 10 seconds...")
			time.Sleep(10 * time.Second)
			continue // Retry the loop, do NOT exit
		}

		fmt.Printf("Got %d peers from tracker\n", len(peers))

		// Start workers for all available peers
		var wg sync.WaitGroup
		for _, pe := range peers {
			wg.Add(1)
			go worker(pe, pm, t, writer, &wg)
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
