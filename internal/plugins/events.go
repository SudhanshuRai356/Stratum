package plugins

import (
	"sync"
	"time"
)

// EventBus manages event distribution to plugins
type EventBus struct {
	mu          sync.RWMutex
	subscribers map[EventType][]EventHandler
	eventQueue  chan Event
	running     bool
	stopCh      chan struct{}
}

// EventHandler represents a function that can handle events
type EventHandler func(event Event) error

// NewEventBus creates a new event bus
func NewEventBus() *EventBus {
	return &EventBus{
		subscribers: make(map[EventType][]EventHandler),
		eventQueue:  make(chan Event, 1000), // Buffer for 1000 events
		stopCh:      make(chan struct{}),
	}
}

// Start starts the event bus
func (eb *EventBus) Start() {
	eb.mu.Lock()
	defer eb.mu.Unlock()

	if eb.running {
		return
	}

	eb.running = true
	go eb.processEvents()
}

// Stop stops the event bus
func (eb *EventBus) Stop() {
	eb.mu.Lock()
	defer eb.mu.Unlock()

	if !eb.running {
		return
	}

	close(eb.stopCh)
	eb.running = false
}

// Subscribe adds an event handler for a specific event type
func (eb *EventBus) Subscribe(eventType EventType, handler EventHandler) {
	eb.mu.Lock()
	defer eb.mu.Unlock()

	eb.subscribers[eventType] = append(eb.subscribers[eventType], handler)
}

// Unsubscribe removes an event handler (this is a simplified implementation)
func (eb *EventBus) Unsubscribe(eventType EventType) {
	eb.mu.Lock()
	defer eb.mu.Unlock()

	delete(eb.subscribers, eventType)
}

// Publish publishes an event to the event bus
func (eb *EventBus) Publish(event Event) {
	select {
	case eb.eventQueue <- event:
	default:
		// Event queue is full, drop the event (or implement a different strategy)
	}
}

// processEvents processes events from the queue
func (eb *EventBus) processEvents() {
	for {
		select {
		case event := <-eb.eventQueue:
			eb.handleEvent(event)
		case <-eb.stopCh:
			return
		}
	}
}

// handleEvent handles a single event by notifying all subscribers
func (eb *EventBus) handleEvent(event Event) {
	eb.mu.RLock()
	handlers := make([]EventHandler, len(eb.subscribers[event.Type]))
	copy(handlers, eb.subscribers[event.Type])
	eb.mu.RUnlock()

	// Execute handlers concurrently
	for _, handler := range handlers {
		go func(h EventHandler) {
			defer func() {
				if r := recover(); r != nil {
					// Log panic recovery
				}
			}()
			h(event)
		}(handler)
	}
}

// EventHelper provides utility functions for creating events
type EventHelper struct{}

// NewEventHelper creates a new event helper
func NewEventHelper() *EventHelper {
	return &EventHelper{}
}

// CreateTorrentEvent creates a torrent-related event
func (eh *EventHelper) CreateTorrentEvent(eventType EventType, torrentData TorrentEvent) Event {
	return Event{
		Type:      eventType,
		Timestamp: time.Now().Unix(),
		Data: map[string]interface{}{
			"torrent": torrentData,
		},
	}
}

// CreatePeerEvent creates a peer-related event
func (eh *EventHelper) CreatePeerEvent(eventType EventType, peerData PeerEvent) Event {
	return Event{
		Type:      eventType,
		Timestamp: time.Now().Unix(),
		Data: map[string]interface{}{
			"peer": peerData,
		},
	}
}

// CreateSystemEvent creates a system-related event
func (eh *EventHelper) CreateSystemEvent(eventType EventType, systemData map[string]interface{}) Event {
	return Event{
		Type:      eventType,
		Timestamp: time.Now().Unix(),
		Data:      systemData,
	}
}