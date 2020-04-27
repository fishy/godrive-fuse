package gdrive_test

import (
	"testing"
	"testing/quick"

	"github.com/fishy/godrive-fuse/gdrive"
)

func TestTraceID(t *testing.T) {
	const expectedLength = 16
	seen := make(map[gdrive.TraceID]bool)
	f := func() bool {
		// This function checks the following things:
		// 1. NewTraceID doesn't block indefinitely and always return non-zero
		// 2. NewTraceID returns a different value from the last call
		// 3. id.String() always return the same length
		id := gdrive.NewTraceID()
		t.Log("id:", id)
		if id == gdrive.TraceID(0) {
			t.Errorf("Expected non-zero trace id, got 0")
		}
		str := id.String()
		if len(str) != expectedLength {
			t.Errorf("Expected string with length %d, got %q", expectedLength, str)
		}
		if seen[id] {
			t.Errorf("This id %v was seen before", id)
		}
		seen[id] = true
		return !t.Failed()
	}
	if err := quick.Check(f, nil); err != nil {
		t.Error(err)
	}
}
