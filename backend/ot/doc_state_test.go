package ot

import (
	"errors"
	"strings"
	"testing"
)

func TestFailedPersistenceDoesNotAdvanceDocument(t *testing.T) {
	state := NewDocumentState("original", 0)
	diskErr := errors.New("database unavailable")
	_, _, err := state.ApplyClientOpsAndPersist(0, "client", []Operation{{Type: OpInsert, Pos: 8, Text: "!"}}, true,
		func(content string, rev int, ops []Operation) error {
			if content != "original!" || rev != 1 || len(ops) != 1 {
				t.Fatal("invalid pending revision")
			}
			return diskErr
		})
	content, rev := state.GetSnapshot()
	if !errors.Is(err, diskErr) || content != "original" || rev != 0 || len(state.History) != 0 {
		t.Fatalf("failed write changed state: %q rev=%d err=%v", content, rev, err)
	}
}

func TestInvalidBatchDoesNotAdvanceDocument(t *testing.T) {
	for _, tc := range []struct {
		rev int
		ops []Operation
	}{
		{-1, []Operation{{Type: OpInsert, Text: "x"}}}, {0, nil},
		{0, []Operation{{Type: OpInsert, Text: "valid"}, {Type: OpInsert, Pos: -1, Text: "bad"}}},
	} {
		state := NewDocumentState("", 0)
		if _, _, err := state.ApplyClientOps(tc.rev, "client", tc.ops, true); err == nil {
			t.Fatal("invalid batch accepted")
		}
		content, rev := state.GetSnapshot()
		if content != "" || rev != 0 {
			t.Fatal("invalid batch changed document")
		}
	}
}

func TestHistoryLimitsReleaseOldOperations(t *testing.T) {
	state := NewDocumentState("", 0)
	text := strings.Repeat("x", 1024*1024)
	for i := 0; i < 5; i++ {
		ops := []Operation{{Type: OpInsert, Text: text}}
		if i > 0 {
			ops = append([]Operation{{Type: OpDelete, Len: len(text)}}, ops...)
		}
		if _, _, err := state.ApplyClientOps(i, "client", ops, true); err != nil {
			t.Fatal(err)
		}
	}
	if len(state.History) != 4 || state.historyBytes > 4*1024*1024 {
		t.Fatalf("history not bounded: len=%d bytes=%d", len(state.History), state.historyBytes)
	}
	if _, err := state.GetOpsSince(0); err == nil {
		t.Fatal("pruned history must require resync")
	}
}
