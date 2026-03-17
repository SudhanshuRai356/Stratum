package app

import (
	"sync"
	"time"

	"github.com/Kushal-39/Stratum/internal/api"
)

type EventType string

const (
	EventState          EventType = "state"
	EventPieceCompleted EventType = "piece.completed"
	EventPieceFailed    EventType = "piece.failed"
	EventPeerBanned     EventType = "peer.banned"
	EventInfo           EventType = "info"
	EventError          EventType = "error"
	EventRound          EventType = "round"
)

type EventSeverity string

const (
	SeverityInfo  EventSeverity = "info"
	SeverityWarn  EventSeverity = "warn"
	SeverityError EventSeverity = "error"
)

type RuntimeEvent struct {
	Type        EventType        `json:"type"`
	Timestamp   time.Time        `json:"timestamp"`
	State       api.TorrentState `json:"state"`
	Severity    EventSeverity    `json:"severity,omitempty"`
	Message     string           `json:"message,omitempty"`
	PieceIndex  uint32           `json:"piece_index,omitempty"`
	Peer        string           `json:"peer,omitempty"`
	Round       int              `json:"round,omitempty"`
	Completed   int              `json:"completed,omitempty"`
	Total       int              `json:"total,omitempty"`
	ActivePeers int              `json:"active_peers,omitempty"`
	DownloadBPS float64          `json:"download_bps,omitempty"`
	UploadBPS   float64          `json:"upload_bps,omitempty"`
}

type EventBus struct {
	mu          sync.RWMutex
	subscribers map[chan RuntimeEvent]struct{}
	bufferSize  int
}

func NewEventBus(bufferSize int) *EventBus {
	if bufferSize <= 0 {
		bufferSize = 64
	}
	return &EventBus{
		subscribers: make(map[chan RuntimeEvent]struct{}),
		bufferSize:  bufferSize,
	}
}

func (b *EventBus) Subscribe() (<-chan RuntimeEvent, func()) {
	ch := make(chan RuntimeEvent, b.bufferSize)

	b.mu.Lock()
	b.subscribers[ch] = struct{}{}
	b.mu.Unlock()

	unsub := func() {
		b.mu.Lock()
		if _, ok := b.subscribers[ch]; ok {
			delete(b.subscribers, ch)
			close(ch)
		}
		b.mu.Unlock()
	}

	return ch, unsub
}

func (b *EventBus) Publish(evt RuntimeEvent) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	for ch := range b.subscribers {
		select {
		case ch <- evt:
		default:
		}
	}
}
