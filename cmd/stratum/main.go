package main

import (
	"bytes"
	"container/heap"
	"context"
	"crypto/sha1"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"math/rand"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
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

type startedDownload struct {
	key           string
	infoHash      string
	name          string
	totalBytes    int64
	outputPath    string
	stateFile     string
	state         string
	lastTick      time.Time
	lastSizeBytes int64
	current       api.TorrentState
	cmd           *exec.Cmd
}

const childStateEnv = "STRATUM_CHILD_STATE_FILE"

type startedDownloadManager struct {
	mu    sync.Mutex
	items map[string]*startedDownload
}

func newStartedDownloadManager() *startedDownloadManager {
	return &startedDownloadManager{items: make(map[string]*startedDownload)}
}

func matchesDownload(item *startedDownload, query string) bool {
	if strings.TrimSpace(query) == "" {
		return false
	}
	q := strings.TrimSpace(query)
	return item.key == q || item.infoHash == q
}

func (m *startedDownloadManager) stopByInfoHash(infoHash string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, item := range m.items {
		if !matchesDownload(item, infoHash) {
			continue
		}

		if processAlive(item.cmd) && item.cmd.Process != nil {
			if err := item.cmd.Process.Kill(); err != nil {
				return "", fmt.Errorf("failed to stop %s: %w", item.name, err)
			}
		}

		item.state = "Stopped"
		item.current.State = "Stopped"
		item.current.DownloadSpeed = 0
		return item.name, nil
	}

	return "", fmt.Errorf("no managed download found for %s", infoHash)
}

func processAlive(cmd *exec.Cmd) bool {
	if cmd == nil || cmd.Process == nil {
		return false
	}
	err := cmd.Process.Signal(syscall.Signal(0))
	return err == nil
}

func readFileSize(path string) int64 {
	if strings.TrimSpace(path) == "" {
		return 0
	}
	info, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return info.Size()
}

func writeChildState(path string, state api.TorrentState) error {
	if strings.TrimSpace(path) == "" {
		return nil
	}

	payload, err := json.Marshal(state)
	if err != nil {
		return err
	}

	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, payload, 0o600); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}

func readChildState(path string) (api.TorrentState, bool) {
	if strings.TrimSpace(path) == "" {
		return api.TorrentState{}, false
	}

	buf, err := os.ReadFile(path)
	if err != nil {
		return api.TorrentState{}, false
	}

	var state api.TorrentState
	if err := json.Unmarshal(buf, &state); err != nil {
		return api.TorrentState{}, false
	}

	if strings.TrimSpace(state.Name) == "" {
		return api.TorrentState{}, false
	}

	return state, true
}

func clampProgress(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 100 {
		return 100
	}
	return value
}

func resolveStartMetadata(source string, outputDir string) (string, string, string, int64, string, error) {
	trimmed := strings.TrimSpace(source)
	if trimmed == "" {
		return "", "", "", 0, "", fmt.Errorf("source is required")
	}

	if isMagnetLink(trimmed) {
		mag, err := torrent.Parse_Mag_URL(trimmed)
		if err != nil {
			return "", "", "", 0, "", fmt.Errorf("invalid magnet link: %w", err)
		}

		infoHash := ""
		if len(mag.Info_hash) == 20 {
			infoHash = hex.EncodeToString(mag.Info_hash)
		}

		name := strings.TrimSpace(mag.Name)
		if name == "" {
			if infoHash != "" {
				name = "magnet-" + infoHash[:12]
			} else {
				name = "magnet-download"
			}
		}

		key := infoHash
		if key == "" {
			key = trimmed
		}

		return key, infoHash, name, 0, "", nil
	}

	t, err := torrent.ParseTorrent(trimmed)
	if err != nil {
		return "", "", "", 0, "", fmt.Errorf("failed to parse torrent input: %w", err)
	}

	absOutDir, err := filepath.Abs(outputDir)
	if err != nil {
		return "", "", "", 0, "", err
	}

	safeName, err := storage.SanitizeName(t.Info.Name)
	if err != nil {
		return "", "", "", 0, "", err
	}

	infoHash := hex.EncodeToString(t.InfoHash[:])
	key := infoHash
	if key == "" {
		key = trimmed
	}

	return key, infoHash, t.Info.Name, t.Info.Length, filepath.Join(absOutDir, safeName), nil
}

func startManagedDownload(source string, outputDir string, manager *startedDownloadManager) (*startedDownload, error) {
	if manager == nil {
		return nil, fmt.Errorf("manager unavailable")
	}

	normalizedSource, err := normalizeStartSource(source)
	if err != nil {
		return nil, err
	}

	key, infoHash, name, totalBytes, outputPath, err := resolveStartMetadata(normalizedSource, outputDir)
	if err != nil {
		return nil, err
	}

	manager.mu.Lock()
	if existing, ok := manager.items[key]; ok {
		if processAlive(existing.cmd) {
			manager.mu.Unlock()
			return nil, fmt.Errorf("download already running for %s", existing.current.Name)
		}
	}
	manager.mu.Unlock()

	stateFile := filepath.Join(os.TempDir(), fmt.Sprintf("stratum-child-state-%d-%d.json", time.Now().UnixNano(), rand.Intn(1000000)))
	cmd, err := startDetachedDownload(normalizedSource, outputDir, stateFile)
	if err != nil {
		return nil, err
	}

	stateName := strings.TrimSpace(name)
	if stateName == "" || stateName == "." {
		stateName = filepath.Base(outputPath)
	}
	if stateName == "" || stateName == "." {
		stateName = "download"
	}

	entry := &startedDownload{
		key:           key,
		infoHash:      infoHash,
		name:          stateName,
		totalBytes:    totalBytes,
		outputPath:    outputPath,
		stateFile:     stateFile,
		state:         "Starting",
		lastTick:      time.Now(),
		lastSizeBytes: readFileSize(outputPath),
		cmd:           cmd,
		current: api.TorrentState{
			InfoHash:       infoHash,
			Name:           stateName,
			Progress:       0,
			DownloadSpeed:  0,
			State:          "Starting",
			ConnectedPeers: 0,
		},
	}

	manager.mu.Lock()
	manager.items[key] = entry
	manager.mu.Unlock()

	go func(d *startedDownload) {
		_ = d.cmd.Wait()
	}(entry)

	return entry, nil
}

func (m *startedDownloadManager) snapshotAndUpdate() []api.TorrentState {
	m.mu.Lock()
	defer m.mu.Unlock()

	out := make([]api.TorrentState, 0, len(m.items))
	now := time.Now()
	for _, item := range m.items {
		if childState, ok := readChildState(item.stateFile); ok {
			if item.infoHash != "" && strings.TrimSpace(childState.InfoHash) == "" {
				childState.InfoHash = item.infoHash
			}
			if strings.TrimSpace(childState.Name) == "" {
				childState.Name = item.name
			}
			item.current = childState
			item.state = childState.State
			out = append(out, item.current)
			if !processAlive(item.cmd) && item.current.State == "" {
				item.current.State = "Stopped"
			}
			continue
		}

		currentSize := readFileSize(item.outputPath)
		elapsed := now.Sub(item.lastTick).Seconds()
		if elapsed <= 0 {
			elapsed = 1
		}

		delta := currentSize - item.lastSizeBytes
		if delta < 0 {
			delta = 0
		}

		speed := float64(delta) / elapsed
		progress := 0.0
		if item.totalBytes > 0 {
			progress = clampProgress((float64(currentSize) / float64(item.totalBytes)) * 100)
		}

		alive := processAlive(item.cmd)
		state := "Downloading"
		if !alive {
			if item.totalBytes > 0 && currentSize >= item.totalBytes {
				state = "Seeding"
			} else {
				state = "Stopped"
			}
		} else if item.totalBytes > 0 && currentSize >= item.totalBytes {
			state = "Seeding"
		}

		if state != "Downloading" {
			speed = 0
		}

		if item.infoHash != "" {
			item.current.InfoHash = item.infoHash
		} else {
			item.current.InfoHash = item.key
		}
		item.current.Name = item.name
		item.current.Progress = progress
		item.current.DownloadSpeed = speed
		item.current.State = state
		item.current.ConnectedPeers = 0

		item.lastTick = now
		item.lastSizeBytes = currentSize
		item.state = state

		out = append(out, item.current)
	}

	return out
}

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

func openPathInSystem(path string) error {
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("empty path")
	}

	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", path)
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", "", path)
	default:
		cmd = exec.Command("xdg-open", path)
	}

	return cmd.Start()
}

func isMagnetLink(input string) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(input)), "magnet:?")
}

func sanitizeSourceInput(input string) string {
	trimmed := strings.TrimSpace(input)
	trimmed = strings.Trim(trimmed, "\"'`")
	trimmed = strings.TrimSpace(trimmed)
	if strings.HasPrefix(trimmed, "<") && strings.HasSuffix(trimmed, ">") {
		trimmed = strings.TrimSpace(trimmed[1 : len(trimmed)-1])
	}
	return trimmed
}

func isTorrentInput(input string) bool {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return false
	}

	lower := strings.ToLower(trimmed)
	if strings.HasSuffix(lower, ".torrent") {
		return true
	}

	parsed, err := url.Parse(trimmed)
	if err != nil {
		return false
	}

	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return false
	}

	return strings.HasSuffix(strings.ToLower(parsed.Path), ".torrent")
}

func fetchTorrentURLToTemp(sourceURL string) (string, error) {
	const maxTorrentBytes int64 = 16 << 20

	client := &http.Client{Timeout: 30 * time.Second}
	req, err := http.NewRequest(http.MethodGet, sourceURL, nil)
	if err != nil {
		return "", fmt.Errorf("invalid torrent url: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status %d", resp.StatusCode)
	}

	file, err := os.CreateTemp("", "stratum-remote-*.torrent")
	if err != nil {
		return "", fmt.Errorf("failed to create temporary torrent file: %w", err)
	}

	defer func() {
		_ = file.Close()
	}()

	limitedReader := &io.LimitedReader{R: resp.Body, N: maxTorrentBytes + 1}
	written, err := io.Copy(file, limitedReader)
	if err != nil {
		_ = os.Remove(file.Name())
		return "", fmt.Errorf("failed to read torrent response: %w", err)
	}

	if written == 0 {
		_ = os.Remove(file.Name())
		return "", fmt.Errorf("empty torrent file response")
	}

	if written > maxTorrentBytes {
		_ = os.Remove(file.Name())
		return "", fmt.Errorf("torrent file is too large")
	}

	return file.Name(), nil
}

func normalizeStartSource(source string) (string, error) {
	trimmed := sanitizeSourceInput(source)
	if trimmed == "" {
		return "", fmt.Errorf("source is required")
	}

	if isMagnetLink(trimmed) {
		return trimmed, nil
	}

	if !isTorrentInput(trimmed) {
		return "", fmt.Errorf("source must be a magnet link or .torrent path/url")
	}

	parsed, err := url.Parse(trimmed)
	if err != nil {
		return trimmed, nil
	}

	if parsed.Scheme == "http" || parsed.Scheme == "https" {
		path, fetchErr := fetchTorrentURLToTemp(trimmed)
		if fetchErr != nil {
			return "", fmt.Errorf("failed to fetch torrent url: %w", fetchErr)
		}
		return path, nil
	}

	return trimmed, nil
}

func validateStartSource(input string) error {
	trimmed := sanitizeSourceInput(input)
	if trimmed == "" {
		return fmt.Errorf("source is required")
	}
	if isMagnetLink(trimmed) || isTorrentInput(trimmed) {
		return nil
	}
	return fmt.Errorf("source must be a magnet link or .torrent path/url")
}

func writeUploadedTorrentToTemp(encodedTorrent string) (string, error) {
	trimmed := strings.TrimSpace(encodedTorrent)
	if trimmed == "" {
		return "", fmt.Errorf("empty torrent data")
	}

	decoded, err := base64.StdEncoding.DecodeString(trimmed)
	if err != nil {
		return "", fmt.Errorf("invalid torrent file encoding: %w", err)
	}

	file, err := os.CreateTemp("", "stratum-upload-*.torrent")
	if err != nil {
		return "", fmt.Errorf("failed to create temporary torrent file: %w", err)
	}
	defer file.Close()

	if _, err := file.Write(decoded); err != nil {
		return "", fmt.Errorf("failed to write temporary torrent file: %w", err)
	}

	return file.Name(), nil
}

func startDetachedDownload(source string, outputDir string, stateFile string) (*exec.Cmd, error) {
	executablePath, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("failed to resolve executable path: %w", err)
	}

	cmd := exec.Command(executablePath, "-o", outputDir, source)
	cmd.Env = append(os.Environ(), fmt.Sprintf("%s=%s", childStateEnv, stateFile))

	if err := cmd.Start(); err != nil {
		return nil, err
	}

	return cmd, nil
}

func runAPIDaemonOnly(outputDir string) error {
	// Initialize plugin system
	if err := initializePluginSystem(outputDir); err != nil {
		return fmt.Errorf("failed to initialize plugin system: %w", err)
	}

	// Start plugins
	if err := startPlugins(); err != nil {
		return fmt.Errorf("failed to start plugins: %w", err)
	}

	fmt.Println("No initial torrent provided. Running in API-only mode.")
	fmt.Println("Use frontend input bar to start magnet or .torrent downloads.")
	fmt.Printf("API token: %s\n", api.GetAPIToken())
	fmt.Println("Connect via: ws://127.0.0.1:8080/ws?token=" + api.GetAPIToken())

	hub := api.NewHub()
	go hub.Run()
	manager := newStartedDownloadManager()

	go func() {
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			states := manager.snapshotAndUpdate()
			for _, state := range states {
				select {
				case hub.Broadcast <- state:
				default:
				}
			}
		}
	}()

	return api.ListenAndServeWithConfig(api.ServerConfig{
		Addr: "127.0.0.1:8080",
		Hub:  hub,
		PluginManager: getPluginManager(),
		HandleControl: func(payload api.ControlPayload) (string, error) {
			action := strings.ToLower(strings.TrimSpace(payload.Action))
			if action != "stop" {
				return "", fmt.Errorf("only stop is available in daemon mode")
			}

			name, err := manager.stopByInfoHash(payload.InfoHash)
			if err != nil {
				return "", err
			}
			return fmt.Sprintf("stopped %s", name), nil
		},
		HandleStart: func(payload api.StartPayload) (string, error) {
			source := strings.TrimSpace(payload.Source)
			if source == "" {
				tempPath, err := writeUploadedTorrentToTemp(payload.TorrentBase64)
				if err != nil {
					return "", err
				}
				source = tempPath
			}

			if err := validateStartSource(source); err != nil {
				return "", err
			}

			download, err := startManagedDownload(source, outputDir, manager)
			if err != nil {
				return "", fmt.Errorf("failed to start download: %w", err)
			}

			if payload.Filename != "" {
				return fmt.Sprintf("download process started for %s", payload.Filename), nil
			}
			return fmt.Sprintf("download process started for %s", download.name), nil
		},
		GetState: func() []api.TorrentState {
			return manager.snapshotAndUpdate()
		},
	})
}
func main() {
	rand.Seed(time.Now().UnixNano())

	outDir := flag.String("o", ".", "output directory")
	uiMode := flag.String("ui", "cli", "ui mode: cli or tui")
	flag.Parse()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if flag.NArg() < 1 {
		if err := runAPIDaemonOnly(*outDir); err != nil {
			fmt.Fprintf(os.Stderr, "failed to start API daemon: %v\n", err)
			os.Exit(1)
		}
		return
	}

	input := strings.TrimSpace(flag.Arg(0))
	var t *torrent.Torrent
	var err error

	// Detect if input is magnet link or torrent file
	if isMagnetLink(input) {
		fmt.Println("Detected magnet link, fetching metadata...")
		t, err = handleMagnetLink(input)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to get metadata from magnet link: %v\n", err)
			os.Exit(1)
		}
	} else {
		if !isTorrentInput(input) {
			fmt.Fprintf(os.Stderr, "invalid input: %q\n", input)
			fmt.Fprintln(os.Stderr, "expected a .torrent file path or a magnet URI")
			fmt.Fprintln(os.Stderr, "example: go run ./cmd/stratum --ui=tui ./archlinux-2026.03.01-x86_64.iso.torrent")
			os.Exit(1)
		}

		normalizedInput, normalizeErr := normalizeStartSource(input)
		if normalizeErr != nil {
			fmt.Fprintf(os.Stderr, "failed to prepare torrent input %q: %v\n", input, normalizeErr)
			os.Exit(1)
		}

		// Parse torrent file
		t, err = torrent.ParseTorrent(normalizedInput)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to parse torrent file %q: %v\n", normalizedInput, err)
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
		InfoHash:        fmt.Sprintf("%x", t.InfoHash),
		Name:            t.Info.Name,
		Progress:        initialProgress,
		State:           "Downloading",
		ConnectedPeers:  len(peers),
		Seeders:         0,
		Leechers:        0,
		CompletedPieces: completedCount,
		TotalPieces:     totalPieces,
		Round:           0,
	})
	_ = writeChildState(os.Getenv(childStateEnv), latestState.Load().(api.TorrentState))

	// Initialize plugin system
	if err := initializePluginSystem(*outDir); err != nil {
		fmt.Fprintf(os.Stderr, "failed to initialize plugin system: %v\n", err)
		// Continue anyway - plugins are optional
	} else {
		if err := startPlugins(); err != nil {
			fmt.Fprintf(os.Stderr, "failed to start plugins: %v\n", err)
			// Continue anyway - plugins are optional
		}
	}

	fmt.Println("Starting local API bridge on 127.0.0.1:8080...")
	fmt.Printf("API token: %s\n", api.GetAPIToken())
	fmt.Println("Connect via: ws://127.0.0.1:8080/ws?token=" + api.GetAPIToken())
	hub := api.NewHub()
	go hub.Run()
	startedManager := newStartedDownloadManager()
	go func() {
		if err := api.ListenAndServeWithConfig(api.ServerConfig{
			Addr: "127.0.0.1:8080",
			Hub:  hub,
			PluginManager: getPluginManager(),
			HandleControl: func(payload api.ControlPayload) (string, error) {
				action := strings.ToLower(strings.TrimSpace(payload.Action))
				if action == "stop" && strings.TrimSpace(payload.InfoHash) != "" {
					name, err := startedManager.stopByInfoHash(payload.InfoHash)
					if err == nil {
						return fmt.Sprintf("stopped %s", name), nil
					}
				}

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
				startedStates := startedManager.snapshotAndUpdate()
				v := latestState.Load()
				if v == nil {
					return startedStates
				}
				state, ok := v.(api.TorrentState)
				if !ok {
					return startedStates
				}
				return append([]api.TorrentState{state}, startedStates...)
			},
			HandleOpen: func(payload api.OpenPayload) (string, error) {
				if err := openPathInSystem(outputPath); err != nil {
					return "", fmt.Errorf("failed to open completed path: %w", err)
				}
				if payload.InfoHash != "" {
					return fmt.Sprintf("opened completed output for %s", payload.InfoHash), nil
				}
				return "opened completed output path", nil
			},
			HandleStart: func(payload api.StartPayload) (string, error) {
				source := strings.TrimSpace(payload.Source)
				if source == "" {
					tempPath, err := writeUploadedTorrentToTemp(payload.TorrentBase64)
					if err != nil {
						return "", err
					}
					source = tempPath
				}

				if err := validateStartSource(source); err != nil {
					return "", err
				}

				download, err := startManagedDownload(source, *outDir, startedManager)
				if err != nil {
					return "", fmt.Errorf("failed to start download: %w", err)
				}

				if payload.Filename != "" {
					return fmt.Sprintf("download process started for %s", payload.Filename), nil
				}
				return fmt.Sprintf("download process started for %s", download.name), nil
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
					InfoHash:        fmt.Sprintf("%x", t.InfoHash),
					Name:            t.Info.Name,
					Progress:        progress,
					DownloadSpeed:   downloadBPS,
					UploadSpeed:     uploadBPS,
					State:           status,
					ConnectedPeers:  int(currentPeers.Load()),
					Seeders:         0,
					Leechers:        0,
					CompletedPieces: completed,
					TotalPieces:     total,
					Round:           int(currentRound.Load()),
				}
				seeders, leechers := tracker.LastSwarmStats()
				if seeders > 0 || leechers > 0 {
					state.Seeders = seeders
					state.Leechers = leechers
				}
				latestState.Store(state)
				_ = writeChildState(os.Getenv(childStateEnv), state)

				if hub != nil {
					select {
					case hub.Broadcast <- state:
					default:
					}

					for _, childState := range startedManager.snapshotAndUpdate() {
						select {
						case hub.Broadcast <- childState:
						default:
						}
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
			InfoHash:        fmt.Sprintf("%x", t.InfoHash),
			Name:            t.Info.Name,
			Progress:        100.0,
			State:           "Seeding",
			Seeders:         0,
			Leechers:        0,
			CompletedPieces: totalPieces,
			TotalPieces:     totalPieces,
			Round:           int(currentRound.Load()),
		}
		seeders, leechers := tracker.LastSwarmStats()
		if seeders > 0 || leechers > 0 {
			state.Seeders = seeders
			state.Leechers = leechers
		}
		latestState.Store(state)
		_ = writeChildState(os.Getenv(childStateEnv), state)
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
	stateFile := os.Getenv(childStateEnv)
	if stateFile != "" {
		seeders, leechers := tracker.LastSwarmStats()
		_ = writeChildState(stateFile, api.TorrentState{
			InfoHash:       fmt.Sprintf("%x", infoHash),
			Name:           magnet.Name,
			Progress:       0,
			State:          "Fetching metadata",
			ConnectedPeers: len(peers),
			Seeders:        seeders,
			Leechers:       leechers,
		})
	}

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
