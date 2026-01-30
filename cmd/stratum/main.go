package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/Kushal-39/Stratum/internal/peer"
	"github.com/Kushal-39/Stratum/internal/storage"
	"github.com/Kushal-39/Stratum/internal/torrent"
	"github.com/Kushal-39/Stratum/internal/tracker"
	"sync"
)
type PieceManager struct{ //struct to manage the concurrency in peers
	completed []bool
	done_chain chan uint32
	piece_chain chan uint32 // channel will be used to avoid the race condition of multiple peers send the same piece
	totalPieces int
	mu sync.Mutex
}
func NewPieceManager(totalPieces int) *PieceManager{
	pm := &PieceManager{
		completed: make([]bool,totalPieces),
		piece_chain: make(chan uint32,totalPieces),
		done_chain: make(chan uint32,totalPieces),
		totalPieces: totalPieces,
	}
	for i:=0;i<totalPieces;i++{
		pm.piece_chain <- uint32(i)
	}
	return pm
}
func downloadPiece(session *peer.PeerSession, pidx uint32,tor *torrent.Torrent, writer *storage.Writer) error{
	temp_Comp := make([]bool,len(tor.Info.Pieces)/20)
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
func worker(pe tracker.Peer, pm *PieceManager, tor *torrent.Torrent, writer *storage.Writer){
	pid:=generatePeerID()
	p:=peer.Peer{
		IP:pe.IP,
		Port:pe.Port,
	}
	con,e := peer.PerformHandshake(p,tor.InfoHash, pid)
	if e != nil{
		fmt.Println("Handshake failed with the peer",p.IP,"due to error ",e)
		return
	}
	session := peer.NewPeerSession(con, p, tor.InfoHash, pid)
	for {
		select{
			case pieceIndex,ok := <- pm.piece_chain:
				if !ok{
					fmt.Printf("Worker %s: piece chain closed\n", p.IP)
					return
				}
				e2:=downloadPiece(session, pieceIndex, tor, writer)
				if e2 != nil {
					fmt.Println("Error downloading piece", pieceIndex, ":", e2)
					select{
					case pm.piece_chain <- pieceIndex:
					default:
						//nothing to see here bhai
					}
					continue
				}else{
				pm.done_chain <- pieceIndex
				}
			default:
				fmt.Printf("Worker %s is done no more pieces left on it\n", p.IP)
            	return
		}
	}
}
const PipelineDepth = 5
func (pm *PieceManager) StartCompletionHandler(){
	comp_count:=0
	for pidx:= range pm.done_chain{
		pm.mu.Lock()
		pm.completed[pidx] = true
		comp_count++
		pm.mu.Unlock()
		fmt.Println("completed", comp_count,"picese out of",pm.totalPieces)
		if comp_count == pm.totalPieces{
			fmt.Println("done all the pieces")
			close(pm.piece_chain)
			return
		}
	}
}
func (pm *PieceManager) waitForCompletion(){
	for{
		pm.mu.Lock()
		done :=true
		for _,d:=range pm.completed{
			if !d{
				done = false
				break
			}
		}
		pm.mu.Unlock()
		if done{
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
}
func allPiecesDone(completed []bool) bool {
	for _, done := range completed {
		if !done {
			return false
		}
	}
	return true
}

func main() {
	outDir := flag.String("o", ".", "output directory")
	flag.Parse()

	if flag.NArg() < 1 {
		fmt.Println("Usage: stratum [-o output_dir] <path-to-torrent-file>")
		os.Exit(1)
	}

	torrentPath := flag.Arg(0)

	// Step 1: Parse the torrent file
	t, err := torrent.ParseTorrent(torrentPath)
	if err != nil {
		panic(err)
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

	// for !allPiecesDone(completed) {
	// 	handshakeDone := false
	// 	if len(peers) == 0 {
	// 		fmt.Println("No peers, re-announcing...")
	// 		peers, _ = tracker.Announce(t)
	// 		time.Sleep(5 * time.Second)
	// 		continue
	// 	}

	// 	for i := 0; i < len(peers); i++ {
	// 		p := peer.Peer{
	// 			IP:   peers[i].IP,
	// 			Port: peers[i].Port,
	// 		}

	// 		// Perform handshake using net.Conn
	// 		conn, err := peer.PerformHandshake(p, t.InfoHash, peerID)
	// 		if err != nil {
	// 			fmt.Println("Handshake failed with", p.IP, ":", err)
	// 			continue
	// 		}
	// 		fmt.Println("Handshake successful with", p.IP)
	// 		handshakeDone = true

	// 		// Step 4: Create session and downloader
	// 		session := peer.NewPeerSession(conn, p, t.InfoHash, peerID)

	// 		err = session.Run(
	// 			totalPieces,
	// 			completed,
	// 			func(pieceIndex uint32) *peer.Downloader {
	// 				return peer.NewDownloader(
	// 					t,
	// 					pieceIndex,
	// 					PipelineDepth,
	// 					writer,
	// 				)
	// 			},
	// 		)
	// 		if err != nil {
	// 			fmt.Println("Session error:", err)
	// 		}

	// 		if allPiecesDone(completed) {
	// 			fmt.Println("All pieces downloaded")
	// 			return
	// 		}

	// 		// otherwise, try next peer
	// 		continue

	// 	}

	// 	if !handshakeDone {
	// 		fmt.Println("No successful handshakes")
	// 		time.Sleep(5 * time.Second)
	// 	}
	// 	time.Sleep(2 * time.Second)
	// }
	//commented out but not yet removed the download all from single peer code and adding the concurrency code
	pm := NewPieceManager(totalPieces)
	for _,pe := range peers{
		go worker(pe, pm, t, writer)
	}
	pm.StartCompletionHandler()
	//pm.waitForCompletion()
}

func generatePeerID() [20]byte {
	var id [20]byte
	copy(id[:], "-ST0001-123456789012")
	return id
}
