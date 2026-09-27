package ot

import (
	"testing"
)

func TestApplyInsert(t *testing.T) {
	doc := "Hello World"
	op := Operation{
		Type: OpInsert,
		Pos:  5,
		Text: " Beautiful",
	}

	result, err := Apply(doc, op)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := "Hello Beautiful World"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestApplyDelete(t *testing.T) {
	doc := "Hello Beautiful World"
	op := Operation{
		Type: OpDelete,
		Pos:  5,
		Len:  10,
	}

	result, err := Apply(doc, op)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := "Hello World"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestApplyUnicodeRunes(t *testing.T) {
	// Multi-byte unicode: "🚀 Hello 世界"
	// Runes: [🚀(0), ' '(1), H(2), e(3), l(4), l(5), o(6), ' '(7), 世(8), 界(9)]
	doc := "🚀 Hello 世界"

	// Insert into the middle of unicode characters
	opInsert := Operation{
		Type: OpInsert,
		Pos:  9, // Before '界'
		Text: "Wonderful ",
	}
	result, err := Apply(doc, opInsert)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := "🚀 Hello 世Wonderful 界"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}

	// Delete unicode character (rocket at index 0, len 1 rune)
	opDelete := Operation{
		Type: OpDelete,
		Pos:  0,
		Len:  2, // Delete rocket and space
	}
	resultAfterDelete, err := Apply(result, opDelete)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expectedAfterDelete := "Hello 世Wonderful 界"
	if resultAfterDelete != expectedAfterDelete {
		t.Errorf("expected %q, got %q", expectedAfterDelete, resultAfterDelete)
	}
}

func TestTransformInsertVsInsert(t *testing.T) {
	initial := "Hello World"

	// User A inserts " there" at pos 5
	opA := Operation{Type: OpInsert, Pos: 5, Text: " there"}
	// User B inserts " brave" at pos 5
	opB := Operation{Type: OpInsert, Pos: 5, Text: " brave"}

	// Let A have priority
	aPrime, bPrime := Transform(opA, opB, true)

	// Applying A then B'
	res1, _ := Apply(initial, opA)
	res1, _ = Apply(res1, bPrime)

	// Applying B then A'
	res2, _ := Apply(initial, opB)
	res2, _ = Apply(res2, aPrime)

	if res1 != res2 {
		t.Errorf("Convergence failed! res1: %q, res2: %q", res1, res2)
	}
	t.Logf("Converged result: %q", res1)
}

func TestTransformInsertVsDelete(t *testing.T) {
	initial := "The quick brown fox jumps"

	// Client A inserts " very" at 9 (before brown)
	opA := Operation{Type: OpInsert, Pos: 9, Text: " very"}
	// Client B deletes "brown " (pos 10, len 6)
	opB := Operation{Type: OpDelete, Pos: 10, Len: 6}

	aPrime, bPrime := Transform(opA, opB, true)

	res1, _ := Apply(initial, opA)
	res1, _ = Apply(res1, bPrime)

	res2, _ := Apply(initial, opB)
	res2, _ = Apply(res2, aPrime)

	if res1 != res2 {
		t.Errorf("Convergence failed! res1: %q, res2: %q", res1, res2)
	}
}

func TestTransformDeleteVsDelete(t *testing.T) {
	initial := "ABCDEFGHIJ"

	// Op A deletes "CD" (pos 2, len 2)
	opA := Operation{Type: OpDelete, Pos: 2, Len: 2}
	// Op B deletes "FG" (pos 5, len 2)
	opB := Operation{Type: OpDelete, Pos: 5, Len: 2}

	aPrime, bPrime := Transform(opA, opB, true)

	res1, _ := Apply(initial, opA)
	res1, _ = Apply(res1, bPrime)

	res2, _ := Apply(initial, opB)
	res2, _ = Apply(res2, aPrime)

	if res1 != res2 {
		t.Errorf("Convergence failed! res1: %q, res2: %q", res1, res2)
	}
	expected := "ABEHIJ"
	if res1 != expected {
		t.Errorf("expected %q, got %q", expected, res1)
	}
}

func TestDocumentStateConcurrentEdits(t *testing.T) {
	ds := NewDocumentState("Hello World", 0)

	// Client 1 sends an op based on rev 0: insert " Beautiful" at 5
	rev1, _, err := ds.ApplyClientOps(0, "client-1", []Operation{
		{Type: OpInsert, Pos: 5, Text: " Beautiful"},
	}, true)
	if err != nil {
		t.Fatalf("unexpected error client 1: %v", err)
	}
	if rev1 != 1 {
		t.Fatalf("expected rev 1, got %d", rev1)
	}
	if ds.Content != "Hello Beautiful World" {
		t.Fatalf("unexpected content: %s", ds.Content)
	}

	// Client 2 also prepared an op based on rev 0 (concurrent): delete " World" (pos 5, len 6)
	rev2, transformedOps, err := ds.ApplyClientOps(0, "client-2", []Operation{
		{Type: OpDelete, Pos: 5, Len: 6},
	}, false)
	if err != nil {
		t.Fatalf("unexpected error client 2: %v", err)
	}
	if rev2 != 2 {
		t.Fatalf("expected rev 2, got %d", rev2)
	}

	// In the transformed op, deletion of " World" should now be shifted past " Beautiful" (pos 15, len 6)
	if len(transformedOps) != 1 || transformedOps[0].Pos != 15 {
		t.Errorf("expected transformed op at pos 15, got %+v", transformedOps)
	}
	expected := "Hello Beautiful"
	if ds.Content != expected {
		t.Errorf("expected doc %q, got %q", expected, ds.Content)
	}

	// Verify GetOpsSince
	records, err := ds.GetOpsSince(0)
	if err != nil {
		t.Fatalf("unexpected error GetOpsSince: %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("expected 2 records, got %d", len(records))
	}
}
