package ot

import (
	"errors"
	"fmt"
	"unicode/utf8"
)

// OpType represents the type of text operation
type OpType string

const (
	OpInsert OpType = "insert"
	OpDelete OpType = "delete"
)

// Operation represents an atomic text modification
type Operation struct {
	Type OpType `json:"type"`
	Pos  int    `json:"pos"`            // Rune index in document
	Text string `json:"text,omitempty"` // For insert
	Len  int    `json:"len,omitempty"`  // For delete (in runes)
}

var (
	ErrInvalidOp        = errors.New("invalid operation")
	ErrIndexOutOfBounds = errors.New("operation index out of bounds")
	ErrRevisionMismatch = errors.New("revision mismatch")
)

// Validate validates that an operation is structurally sound
func (op Operation) Validate() error {
	if op.Pos < 0 {
		return fmt.Errorf("%w: position cannot be negative", ErrInvalidOp)
	}
	switch op.Type {
	case OpInsert:
		if op.Text == "" {
			return fmt.Errorf("%w: insert operation text cannot be empty", ErrInvalidOp)
		}
	case OpDelete:
		if op.Len <= 0 {
			return fmt.Errorf("%w: delete operation length must be positive", ErrInvalidOp)
		}
	default:
		return fmt.Errorf("%w: unknown op type %s", ErrInvalidOp, op.Type)
	}
	return nil
}

// Length returns the rune length of an insert or delete operation
func (op Operation) Length() int {
	if op.Type == OpInsert {
		return utf8.RuneCountInString(op.Text)
	}
	return op.Len
}

// Apply applies an operation to a document string and returns the resulting string.
// Calculations are performed on runes (Unicode code points) to ensure UTF-8 safety.
func Apply(doc string, op Operation) (string, error) {
	if err := op.Validate(); err != nil {
		return doc, err
	}

	runes := []rune(doc)
	docLen := len(runes)

	if op.Pos > docLen {
		return doc, fmt.Errorf("%w: pos %d > doc length %d", ErrIndexOutOfBounds, op.Pos, docLen)
	}

	switch op.Type {
	case OpInsert:
		insertRunes := []rune(op.Text)
		newRunes := make([]rune, 0, docLen+len(insertRunes))
		newRunes = append(newRunes, runes[:op.Pos]...)
		newRunes = append(newRunes, insertRunes...)
		newRunes = append(newRunes, runes[op.Pos:]...)
		return string(newRunes), nil

	case OpDelete:
		end := op.Pos + min(op.Len, docLen-op.Pos)
		if op.Pos > end {
			return doc, fmt.Errorf("%w: delete start %d > end %d", ErrIndexOutOfBounds, op.Pos, end)
		}
		newRunes := make([]rune, 0, docLen-(end-op.Pos))
		newRunes = append(newRunes, runes[:op.Pos]...)
		newRunes = append(newRunes, runes[end:]...)
		return string(newRunes), nil

	default:
		return doc, fmt.Errorf("%w: unsupported type %s", ErrInvalidOp, op.Type)
	}
}

// ApplyAll applies a sequence of operations to a document string in order
func ApplyAll(doc string, ops []Operation) (string, error) {
	current := doc
	for _, op := range ops {
		var err error
		current, err = Apply(current, op)
		if err != nil {
			return doc, err
		}
	}
	return current, nil
}

// Transform transforms two concurrent operations a and b against each other.
// Returns (a', b') such that applying a then b' yields the same state as applying b then a'.
// priorityA breaks ties when a and b insert at the exact same position (true if a has higher priority).
func Transform(a, b Operation, priorityA bool) (Operation, Operation) {
	aPrime := a
	bPrime := b

	lenA := a.Length()
	lenB := b.Length()

	switch {
	// Case 1: Insert vs Insert
	case a.Type == OpInsert && b.Type == OpInsert:
		if a.Pos < b.Pos || (a.Pos == b.Pos && priorityA) {
			bPrime.Pos += lenA
		} else {
			aPrime.Pos += lenB
		}

	// Case 2: Insert vs Delete
	case a.Type == OpInsert && b.Type == OpDelete:
		if a.Pos <= b.Pos {
			bPrime.Pos += lenA
		} else if a.Pos >= b.Pos+lenB {
			aPrime.Pos -= lenB
		} else {
			// a inserts inside b's deleted region:
			// The inserted text survives at the delete position.
			aPrime.Pos = b.Pos
			bPrime.Len += lenA
		}

	// Case 3: Delete vs Insert
	case a.Type == OpDelete && b.Type == OpInsert:
		transformedB, transformedA := Transform(b, a, !priorityA)
		return transformedA, transformedB

	// Case 4: Delete vs Delete
	case a.Type == OpDelete && b.Type == OpDelete:
		aEnd := a.Pos + a.Len
		bEnd := b.Pos + b.Len

		if aEnd <= b.Pos {
			// a is entirely before b
			bPrime.Pos -= a.Len
		} else if bEnd <= a.Pos {
			// b is entirely before a
			aPrime.Pos -= b.Len
		} else {
			// Overlapping deletes
			minPos := min(a.Pos, b.Pos)
			maxEnd := max(aEnd, bEnd)

			if a.Pos <= b.Pos && aEnd >= bEnd {
				// a covers b entirely
				bPrime.Len = 0
				aPrime.Len = maxEnd - minPos - b.Len
			} else if b.Pos <= a.Pos && bEnd >= aEnd {
				// b covers a entirely
				aPrime.Len = 0
				bPrime.Len = maxEnd - minPos - a.Len
			} else if a.Pos < b.Pos {
				// a starts before b, b ends after a
				overlap := aEnd - b.Pos
				aPrime.Len = a.Len - overlap
				bPrime.Pos = a.Pos
				bPrime.Len = b.Len - overlap
			} else {
				// b starts before a, a ends after b
				overlap := bEnd - a.Pos
				bPrime.Len = b.Len - overlap
				aPrime.Pos = b.Pos
				aPrime.Len = a.Len - overlap
			}
		}
	}

	return aPrime, bPrime
}

// TransformOps transforms a list of operations from client A against a list of concurrent operations B.
// Returns transformed A operations that can be applied to current document state.
func TransformOps(opsA []Operation, opsB []Operation, priorityA bool) []Operation {
	resA := make([]Operation, len(opsA))
	copy(resA, opsA)

	for _, opB := range opsB {
		nextA := make([]Operation, 0, len(resA))
		currB := opB
		for _, opA := range resA {
			if currB.Len == 0 && currB.Type == OpDelete {
				nextA = append(nextA, opA)
				continue
			}
			transformedA, transformedB := Transform(opA, currB, priorityA)
			if !(transformedA.Type == OpDelete && transformedA.Len <= 0) {
				nextA = append(nextA, transformedA)
			}
			currB = transformedB
		}
		resA = nextA
	}

	return resA
}
