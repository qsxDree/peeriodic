package ot

import (
	"fmt"
	"sync"
	"time"
)

// RevisionRecord captures an applied set of operations at a specific revision
type RevisionRecord struct {
	Rev       int         `json:"rev"`
	ClientID  string      `json:"client_id"`
	Ops       []Operation `json:"ops"`
	Timestamp time.Time   `json:"timestamp"`
}

// DocumentState manages in-memory document state, revisions, and operation history
type DocumentState struct {
	mu         sync.RWMutex
	Content    string
	Rev        int
	History    []RevisionRecord
	MaxHistory int
}

// NewDocumentState initializes document state with initial content and revision
func NewDocumentState(initialContent string, initialRev int) *DocumentState {
	return &DocumentState{
		Content:    initialContent,
		Rev:        initialRev,
		History:    make([]RevisionRecord, 0, 256),
		MaxHistory: 1000,
	}
}

// ApplyClientOps applies operations originating from a client with a given base revision.
// If the client's revision is behind the server's current revision, the operations are
// transformed against all intermediate operations in history.
func (ds *DocumentState) ApplyClientOps(clientRev int, clientID string, ops []Operation, priority bool) (int, []Operation, error) {
	ds.mu.Lock()
	defer ds.mu.Unlock()

	if clientRev > ds.Rev {
		return ds.Rev, nil, fmt.Errorf("%w: client rev %d is ahead of server rev %d", ErrRevisionMismatch, clientRev, ds.Rev)
	}

	for _, op := range ops {
		if err := op.Validate(); err != nil {
			return ds.Rev, nil, fmt.Errorf("invalid op in batch: %w", err)
		}
	}

	transformedOps := ops

	// If client is behind, transform against missed concurrent operations
	if clientRev < ds.Rev {
		historyLen := len(ds.History)
		missedCount := ds.Rev - clientRev
		if missedCount > historyLen {
			return ds.Rev, nil, fmt.Errorf("%w: client revision %d is too old for history buffer (available: %d)", ErrRevisionMismatch, clientRev, ds.Rev-historyLen)
		}

		startIndex := historyLen - missedCount
		for i := startIndex; i < historyLen; i++ {
			concurrentRecord := ds.History[i]
			transformedOps = TransformOps(transformedOps, concurrentRecord.Ops, priority)
		}
	}

	// Apply transformed operations to the server document
	newContent, err := ApplyAll(ds.Content, transformedOps)
	if err != nil {
		return ds.Rev, nil, fmt.Errorf("failed to apply transformed ops: %w", err)
	}

	ds.Content = newContent
	ds.Rev++

	record := RevisionRecord{
		Rev:       ds.Rev,
		ClientID:  clientID,
		Ops:       transformedOps,
		Timestamp: time.Now(),
	}

	ds.History = append(ds.History, record)
	if len(ds.History) > ds.MaxHistory {
		// Prune oldest history
		ds.History = ds.History[len(ds.History)-ds.MaxHistory:]
	}

	return ds.Rev, transformedOps, nil
}

// GetOpsSince retrieves all revision records after `sinceRev`
func (ds *DocumentState) GetOpsSince(sinceRev int) ([]RevisionRecord, error) {
	ds.mu.RLock()
	defer ds.mu.RUnlock()

	if sinceRev == ds.Rev {
		return nil, nil
	}

	if sinceRev > ds.Rev {
		return nil, fmt.Errorf("%w: requested rev %d > current rev %d", ErrRevisionMismatch, sinceRev, ds.Rev)
	}

	historyLen := len(ds.History)
	diff := ds.Rev - sinceRev
	if diff > historyLen {
		return nil, fmt.Errorf("%w: requested rev %d too old, history truncated", ErrRevisionMismatch, sinceRev)
	}

	startIndex := historyLen - diff
	result := make([]RevisionRecord, diff)
	copy(result, ds.History[startIndex:])
	return result, nil
}

// GetSnapshot returns the current document content and revision safely
func (ds *DocumentState) GetSnapshot() (string, int) {
	ds.mu.RLock()
	defer ds.mu.RUnlock()
	return ds.Content, ds.Rev
}
