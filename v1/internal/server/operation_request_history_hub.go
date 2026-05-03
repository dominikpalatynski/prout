package server

import (
	"sync"

	"github.com/dominikpalatynski/toolshed/internal/store/sqlc"
)

type operationRequestHistoryHub struct {
	mu          sync.Mutex
	nextID      int
	subscribers map[int]operationRequestHistorySubscriber
}

type operationRequestHistorySubscriber struct {
	operationRequestID int64
	entries            chan sqlc.OperationRequestHistoryEntries
}

func newOperationRequestHistoryHub() *operationRequestHistoryHub {
	return &operationRequestHistoryHub{
		subscribers: make(map[int]operationRequestHistorySubscriber),
	}
}

// PublishOperationRequestHistoryEntry delivers best-effort live updates for the
// current process. Persisted history remains the source of truth.
func (h *operationRequestHistoryHub) PublishOperationRequestHistoryEntry(
	entry sqlc.OperationRequestHistoryEntries,
) {
	h.mu.Lock()
	defer h.mu.Unlock()

	for _, subscriber := range h.subscribers {
		if subscriber.operationRequestID != entry.OperationRequestID {
			continue
		}

		select {
		case subscriber.entries <- entry:
		default:
		}
	}
}

func (h *operationRequestHistoryHub) Subscribe(
	operationRequestID int64,
) (<-chan sqlc.OperationRequestHistoryEntries, func()) {
	h.mu.Lock()
	defer h.mu.Unlock()

	id := h.nextID
	h.nextID++

	entries := make(chan sqlc.OperationRequestHistoryEntries, 32)
	h.subscribers[id] = operationRequestHistorySubscriber{
		operationRequestID: operationRequestID,
		entries:            entries,
	}

	unsubscribe := func() {
		h.mu.Lock()
		defer h.mu.Unlock()

		subscriber, ok := h.subscribers[id]
		if !ok {
			return
		}

		delete(h.subscribers, id)
		close(subscriber.entries)
	}

	return entries, unsubscribe
}
