package main

import (
	"bytes"
	"container/heap"
	"context"
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
	"sync/atomic"
	"syscall"
	"time"

	"github.com/Kushal-39/Stratum/internal/api"
	"github.com/Kushal-39/Stratum/internal/app"
	"github.com/Kushal-39/Stratum/internal/dht"
	"github.com/Kushal-39/Stratum/internal/peer"
	"github.com/Kushal-39/Stratum/internal/storage"
	"github.com/Kushal-39/Stratum/internal/torrent"
	"github.com/Kushal-39/Stratum/internal/tracker"
	"github.com/Kushal-39/Stratum/internal/ui/tui"
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
func (pm *PieceManager) getProgress() (int, int) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	return pm.completedCount, pm.totalPieces
}
func downloadPiece(session *peer.PeerSession, pidx uint32, tor *torrent.Torrent, writer *storage.Writer) error {
	temp_Comp := make([]bool, tor.NumPieces())
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
	pieceLen := tor.PieceLength(int(pieceIndex))

	// 2. Read the piece data from disk/storage
	buf := make([]byte, pieceLen)
	offset := tor.PieceOffset(int(pieceIndex))

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
	expected := tor.PieceHash(int(pieceIndex))

	if !bytes.Equal(hash[:], expected) {
		return false
	}
	return true
}
func worker(ctx context.Context, pe tracker.Peer, pm *PieceManager, tor *torrent.Torrent, writer *storage.Writer, seeder *peer.Seeder, hub *api.Hub, bus *app.EventBus, wg *sync.WaitGroup) {
	defer wg.Done()
	select {
	case <-ctx.Done():
		return
	default:
	}

	// 1. CHECK REPUTATION BEFORE CONNECTING
	if pm.IsBanned(pe.IP) {
		// fmt.Printf("Skipping banned peer %s\n", pe.IP)
		return
	}

	pid := generatePeerID()
	p := peer.Peer{IP: pe.IP, Port: pe.Port}

	fmt.Printf("Worker %s: Attempting TCP connection and handshake...\n", p.IP)
	con, reserved, e := peer.PerformHandshake(p, tor.InfoHash, pid)
	if e != nil {
		fmt.Printf("Worker %s: Handshake failed: %v\n", p.IP, e)
		return
	}
	fmt.Printf("Worker %s: Handshake successful!\n", p.IP)
	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			_ = con.Close()
		case <-done:
		}
	}()
	defer close(done)
	defer con.Close()

	session := peer.NewPeerSession(con, p, tor.InfoHash, pid, reserved)
	if err := session.WaitForReady(); err != nil {
		return
	}

	pm.update_avail(session.Bitfield)

	for !pm.allDone() {
		select {
		case <-ctx.Done():
			return
		default:
		}

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
			completed, total := pm.getProgress()
			fmt.Printf("Completed & Verified piece %d (%d/%d)\n", piece_index, completed, total)

			if hub != nil {
				progress := float64(completed) / float64(total) * 100.0
				status := "Downloading"
				if completed >= total {
					status = "Seeding"
				}

				state := api.TorrentState{
					InfoHash: fmt.Sprintf("%x", tor.InfoHash),
					Name:     tor.Info.Name,
					Progress: progress,
					State:    status,
				}

				// Non-blocking send. If the websocket is choked, drop the UI update.
				select {
				case hub.Broadcast <- state:
				default:
				}

				if bus != nil {
					bus.Publish(app.RuntimeEvent{
						Type:       app.EventPieceCompleted,
						Timestamp:  time.Now(),
						Severity:   app.SeverityInfo,
						State:      state,
						Message:    fmt.Sprintf("piece completed %d from %s", piece_index, p.IP),
						PieceIndex: piece_index,
						Peer:       p.IP,
						Completed:  completed,
						Total:      total,
					})
				}
			}

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
			if bus != nil {
				bus.Publish(app.RuntimeEvent{
					Type:       app.EventPeerBanned,
					Timestamp:  time.Now(),
					Severity:   app.SeverityWarn,
					PieceIndex: piece_index,
					Peer:       p.IP,
					Message:    fmt.Sprintf("banned peer %s for corrupt piece %d", p.IP, piece_index),
				})
			}
			return // Disconnect immediately
		}
	}
}

const PipelineDepth = 5
const ReannounceInterval = 30 * time.Second

type ActiveRoundCanceller struct {
	mu     sync.Mutex
	cancel context.CancelFunc
}

func (a *ActiveRoundCanceller) Set(fn context.CancelFunc) {
	a.mu.Lock()
	a.cancel = fn
	a.mu.Unlock()
}

func (a *ActiveRoundCanceller) Cancel() {
	a.mu.Lock()
	if a.cancel != nil {
		a.cancel()
	}
	a.mu.Unlock()
}

func (a *ActiveRoundCanceller) Clear() {
	a.mu.Lock()
	a.cancel = nil
	a.mu.Unlock()
}

func main() {
	rand.Seed(time.Now().UnixNano())

	outDir := flag.String("o", ".", "output directory")
	uiMode := flag.String("ui", "cli", "ui mode: cli or tui")
	flag.Parse()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if flag.NArg() < 1 {
		fmt.Println("Usage: stratum [-o output_dir] [--ui=cli|tui] <path-to-.torrent-file-or-magnet-link>")
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
			fmt.Fprintf(os.Stderr, "failed to get metadata from magnet link: %v\n", err)
			os.Exit(1)
		}
	} else {
		if !strings.HasSuffix(strings.ToLower(input), ".torrent") {
			fmt.Fprintf(os.Stderr, "invalid input: %q\n", input)
			fmt.Fprintln(os.Stderr, "expected a .torrent file path or a magnet URI")
			fmt.Fprintln(os.Stderr, "example: go run ./cmd/stratum --ui=tui ./archlinux-2026.03.01-x86_64.iso.torrent")
			os.Exit(1)
		}

		// Parse torrent file
		t, err = torrent.ParseTorrent(input)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to parse torrent file %q: %v\n", input, err)
			os.Exit(1)
		}
	}

	fmt.Println("Torrent parsed:", t.Info.Name)
	fmt.Printf("DEBUG: t.Info.Files length: %d\n", len(t.Info.Files))

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

	safeName, err := storage.SanitizeName(t.Info.Name)
	if err != nil {
		panic(fmt.Sprintf("Torrent has unsafe name: %v", err))
	}

	outputPath := filepath.Join(absOutDir, safeName)
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
		fmt.Println("\nReceived interrupt signal, shutting down...")
		cancel()
	}()

	ticker := time.NewTicker(10 * time.Minute) //interval for automatic saving of dht nodes data
	defer ticker.Stop()
	//subroutine to save using the ticker
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
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
		}
	}()

	// Step 2: Get peers from tracker and DHT
	peers, err := tracker.Announce(t, t.Info.Length)
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
	totalPieces := t.NumPieces()
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

	// --- NEW API SERVER SETUP ---
	eventBus := app.NewEventBus(128)
	controls := make(chan app.ControlCommand, 16)

	var latestState atomic.Value
	initialProgress := 0.0
	if totalPieces > 0 {
		initialProgress = float64(completedCount) / float64(totalPieces) * 100.0
	}
	latestState.Store(api.TorrentState{
		InfoHash:       fmt.Sprintf("%x", t.InfoHash),
		Name:           t.Info.Name,
		Progress:       initialProgress,
		State:          "Downloading",
		ConnectedPeers: len(peers),
	})

	fmt.Println("Starting local API bridge on 127.0.0.1:8080...")
	fmt.Printf("API token: %s\n", api.GetAPIToken())
	fmt.Println("Connect via: ws://127.0.0.1:8080/ws?token=" + api.GetAPIToken())
	hub := api.NewHub()
	go hub.Run()
	go func() {
		if err := api.ListenAndServeWithConfig(api.ServerConfig{
			Addr: "127.0.0.1:8080",
			Hub:  hub,
			HandleControl: func(payload api.ControlPayload) (string, error) {
				action := strings.ToLower(strings.TrimSpace(payload.Action))
				var cmdType app.ControlCommandType
				switch action {
				case "pause":
					cmdType = app.CmdPause
				case "resume":
					cmdType = app.CmdResume
				case "reannounce":
					cmdType = app.CmdReannounce
				case "stop":
					cmdType = app.CmdStop
				default:
					return "", fmt.Errorf("unknown action %q", payload.Action)
				}

				select {
				case controls <- app.ControlCommand{Type: cmdType}:
					if payload.InfoHash != "" {
						return fmt.Sprintf("accepted %s for %s", action, payload.InfoHash), nil
					}
					return fmt.Sprintf("accepted %s", action), nil
				default:
					return "", fmt.Errorf("control queue busy")
				}
			},
			GetState: func() []api.TorrentState {
				v := latestState.Load()
				if v == nil {
					return []api.TorrentState{}
				}
				state, ok := v.(api.TorrentState)
				if !ok {
					return []api.TorrentState{}
				}
				return []api.TorrentState{state}
			},
		}); err != nil {
			fmt.Printf("API Server crashed: %v\n", err)
		}
	}()
	var stdoutRestore *os.File

	if *uiMode == "tui" {
		devNull, openErr := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
		if openErr == nil {
			stdoutRestore = os.Stdout
			os.Stdout = devNull
			defer func() {
				os.Stdout = stdoutRestore
				_ = devNull.Close()
			}()
		} else {
			fmt.Fprintf(os.Stderr, "warning: failed to quiet stdout for TUI mode: %v\n", openErr)
		}

		go func() {
			if err := tui.Run(ctx, eventBus, controls); err != nil {
				fmt.Fprintf(os.Stderr, "tui exited with error: %v\n", err)
				cancel()
			}
		}()
	}

	var pauseMu sync.RWMutex
	paused := false
	isPaused := func() bool {
		pauseMu.RLock()
		defer pauseMu.RUnlock()
		return paused
	}
	activeRound := &ActiveRoundCanceller{}

	var currentRound atomic.Int64
	var currentPeers atomic.Int64
	currentPeers.Store(int64(len(peers)))

	// 3. Pass loaded state to Manager
	pm := NewPieceManager(totalPieces, completedPieces, completedCount)

	saveStateSnapshot := func(reason string) {
		pm.mu.Lock()
		stateCopy := make([]bool, len(pm.completed))
		copy(stateCopy, pm.completed)
		completedNow := pm.completedCount
		totalNow := pm.totalPieces
		pm.mu.Unlock()

		if err := writer.SaveState(stateCopy); err != nil {
			eventBus.Publish(app.RuntimeEvent{
				Type:      app.EventError,
				Timestamp: time.Now(),
				Severity:  app.SeverityError,
				Message:   fmt.Sprintf("failed to save state during %s: %v", reason, err),
			})
			return
		}

		eventBus.Publish(app.RuntimeEvent{
			Type:      app.EventInfo,
			Timestamp: time.Now(),
			Severity:  app.SeverityInfo,
			Message:   fmt.Sprintf("%s: state saved (%d/%d)", reason, completedNow, totalNow),
			Completed: completedNow,
			Total:     totalNow,
		})
	}

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case cmd := <-controls:
				switch cmd.Type {
				case app.CmdPause:
					pauseMu.Lock()
					paused = true
					pauseMu.Unlock()
					activeRound.Cancel()
					saveStateSnapshot("pause")
					eventBus.Publish(app.RuntimeEvent{Type: app.EventInfo, Timestamp: time.Now(), Severity: app.SeverityWarn, Message: "paused: active workers stopped"})
				case app.CmdResume:
					pauseMu.Lock()
					paused = false
					pauseMu.Unlock()
					eventBus.Publish(app.RuntimeEvent{Type: app.EventInfo, Timestamp: time.Now(), Severity: app.SeverityInfo, Message: "resumed: scheduling new workers"})
				case app.CmdReannounce:
					eventBus.Publish(app.RuntimeEvent{Type: app.EventInfo, Timestamp: time.Now(), Severity: app.SeverityInfo, Message: "reannounce requested (applies next round)"})
				case app.CmdStop:
					activeRound.Cancel()
					saveStateSnapshot("stop")
					eventBus.Publish(app.RuntimeEvent{Type: app.EventInfo, Timestamp: time.Now(), Severity: app.SeverityWarn, Message: "stop requested"})
					cancel()
				}
			}
		}
	}()

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

	go func() {
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()

		lastCompleted := completedCount
		lastUploaded := int64(0)

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				completed, total := pm.getProgress()
				deltaPieces := completed - lastCompleted
				if deltaPieces < 0 {
					deltaPieces = 0
				}
				downloadBPS := float64(deltaPieces * t.Info.PieceLength)
				lastCompleted = completed

				uploadBPS := float64(0)
				if seeder != nil {
					uploadedNow := seeder.GetUploaded()
					deltaUpload := uploadedNow - lastUploaded
					if deltaUpload < 0 {
						deltaUpload = 0
					}
					uploadBPS = float64(deltaUpload)
					lastUploaded = uploadedNow
				}

				status := "Downloading"
				progress := 0.0
				if total > 0 {
					progress = float64(completed) / float64(total) * 100.0
				}
				if completed >= total && total > 0 {
					status = "Seeding"
				}

				state := api.TorrentState{
					InfoHash:       fmt.Sprintf("%x", t.InfoHash),
					Name:           t.Info.Name,
					Progress:       progress,
					DownloadSpeed:  downloadBPS,
					State:          status,
					ConnectedPeers: int(currentPeers.Load()),
				}
				latestState.Store(state)

				if hub != nil {
					select {
					case hub.Broadcast <- state:
					default:
					}
				}

				eventBus.Publish(app.RuntimeEvent{
					Type:        app.EventState,
					Timestamp:   time.Now(),
					Severity:    app.SeverityInfo,
					State:       state,
					Round:       int(currentRound.Load()),
					Completed:   completed,
					Total:       total,
					ActivePeers: int(currentPeers.Load()),
					DownloadBPS: downloadBPS,
					UploadBPS:   uploadBPS,
				})
			}
		}
	}()

	roundNum := 0
downloadLoop:

	for !pm.allDone() {
		select {
		case <-ctx.Done():
			break downloadLoop
		default:
		}

		if isPaused() {
			eventBus.Publish(app.RuntimeEvent{Type: app.EventInfo, Timestamp: time.Now(), Severity: app.SeverityWarn, Message: "download paused"})
		}
		for isPaused() {
			select {
			case <-ctx.Done():
				break downloadLoop
			case <-time.After(500 * time.Millisecond):
			}
		}

		roundNum++
		currentRound.Store(int64(roundNum))
		fmt.Printf("\n=== Round %d: Remaining pieces: %d/%d ===\n", roundNum, pm.remainingPieces(), totalPieces)

		// Always announce at the start of each round to get fresh peers
		fmt.Println("Announcing to tracker...")
		leftBytes := int64(pm.remainingPieces()) * int64(t.Info.PieceLength)
		if leftBytes > t.Info.Length {
			leftBytes = t.Info.Length // Sanity cap
		}
		peers, err = tracker.Announce(t, leftBytes)
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
			currentPeers.Store(0)
			fmt.Println("No peers returned by tracker or DHT. Retrying in 10 seconds...")
			time.Sleep(10 * time.Second)
			continue // Retry the loop, do NOT exit
		}

		currentPeers.Store(int64(len(peers)))
		fmt.Printf("Total peers this round: %d\n", len(peers))

		// Start workers for all available peers
		var wg sync.WaitGroup
		roundCtx, roundCancel := context.WithCancel(ctx)
		activeRound.Set(roundCancel)
		for _, pe := range peers {
			wg.Add(1)
			go worker(roundCtx, pe, pm, t, writer, seeder, hub, eventBus, &wg)
		}

		// Wait for all workers to finish this round
		// Worker exit does NOT end the program - we just wait and loop again
		wg.Wait()
		roundCancel()
		activeRound.Clear()

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

	if ctx.Err() != nil && !pm.allDone() {
		fmt.Println("Download interrupted before completion")
	}
	fmt.Println("\n=== All pieces downloaded successfully! ===")

	// Save DHT nodes immediately
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

	fmt.Println("Entering perpetual seeding mode. Press Ctrl+C to exit.")

	// Force a final UI update
	if hub != nil {
		state := api.TorrentState{
			InfoHash: fmt.Sprintf("%x", t.InfoHash),
			Name:     t.Info.Name,
			Progress: 100.0,
			State:    "Seeding",
		}
		latestState.Store(state)
		select {
		case hub.Broadcast <- state:
		default:
		}
		eventBus.Publish(app.RuntimeEvent{Type: app.EventState, Timestamp: time.Now(), Severity: app.SeverityInfo, State: state, Message: "entered seeding mode", Completed: totalPieces, Total: totalPieces, ActivePeers: int(currentPeers.Load())})
	}

	// Keep process alive and stay in the tracker's peer list
	seedTicker := time.NewTicker(30 * time.Minute)
	defer seedTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			if seeder != nil {
				seeder.Stop()
			}
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
			fmt.Println("Shutdown complete")
			return
		case <-seedTicker.C:
			fmt.Println("Seeding: Re-announcing to swarm...")
			_, _ = tracker.Announce(t, 0)
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

	peers, err := tracker.Announce(tempTorrent, 0)
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
