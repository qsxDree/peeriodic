package services

import (
	"testing"

	"github.com/logoes0/peeriodic.git/config"
	"github.com/logoes0/peeriodic.git/ot"
)

func TestWebSocketServiceRoomManagement(t *testing.T) {
	cfg := &config.Config{
		WebSocket: config.WebSocketConfig{
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
		},
	}

	ws := NewWebSocketService(cfg, nil)
	if ws == nil {
		t.Fatal("expected WebSocketService instance, got nil")
	}

	room := ws.getOrCreateRoom("test-room", "Hello World", 0)
	if room == nil {
		t.Fatal("expected room instance, got nil")
	}

	if room.ID != "test-room" {
		t.Errorf("expected room ID test-room, got %s", room.ID)
	}

	content, rev := room.DocState.GetSnapshot()
	if content != "Hello World" {
		t.Errorf("expected content 'Hello World', got %s", content)
	}
	if rev != 0 {
		t.Errorf("expected rev 0, got %d", rev)
	}

	// Apply an operation via DocState
	newRev, transformed, err := room.DocState.ApplyClientOps(0, "client-1", []ot.Operation{
		{Type: ot.OpInsert, Pos: 11, Text: "!"},
	}, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if newRev != 1 {
		t.Errorf("expected rev 1, got %d", newRev)
	}
	if len(transformed) != 1 {
		t.Errorf("expected 1 transformed op, got %d", len(transformed))
	}

	updatedContent, _ := room.DocState.GetSnapshot()
	if updatedContent != "Hello World!" {
		t.Errorf("expected 'Hello World!', got %q", updatedContent)
	}

	// Test GetPresenceList
	presence := room.GetPresenceList()
	if len(presence) != 0 {
		t.Errorf("expected 0 clients initially, got %d", len(presence))
	}

	// Test Room Stats
	stats := ws.GetRoomStats()
	if count, exists := stats["test-room"]; !exists || count != 0 {
		t.Errorf("expected test-room with 0 count, got count %d, exists: %v", count, exists)
	}
}

func TestLegacyDocumentUpdate(t *testing.T) {
	cfg := &config.Config{}
	ws := NewWebSocketService(cfg, nil)
	room := ws.getOrCreateRoom("legacy-room", "Initial", 0)

	session := &ClientSession{
		ClientID: "test-client",
		Name:     "Test",
		Role:     RoleEditor,
	}

	// Legacy update simulates full text replace
	ws.handleLegacyDocumentUpdate(session, room, "Replaced Content", nil)

	content, rev := room.DocState.GetSnapshot()
	if content != "Replaced Content" {
		t.Errorf("expected 'Replaced Content', got %q", content)
	}
	if rev != 1 {
		t.Errorf("expected rev 1, got %d", rev)
	}
}

func TestViewerCannotEdit(t *testing.T) {
	cfg := &config.Config{}
	ws := NewWebSocketService(cfg, nil)
	room := ws.getOrCreateRoom("viewer-room", "Protected Document", 0)

	viewerSession := &ClientSession{
		ClientID: "viewer-client",
		Name:     "Viewer Bob",
		Role:     RoleViewer,
	}

	// Attempt to edit document as viewer
	ws.handleLegacyDocumentUpdate(viewerSession, room, "Hacked Content", nil)

	content, rev := room.DocState.GetSnapshot()
	if content != "Protected Document" {
		t.Errorf("viewer should not be able to modify content, got %q", content)
	}
	if rev != 0 {
		t.Errorf("expected rev to stay 0, got %d", rev)
	}
}
