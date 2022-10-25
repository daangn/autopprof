package autopprof

import (
	"testing"
)

func TestProfileHeap(t *testing.T) {
	b, err := profileHeap()
	if err != nil {
		t.Errorf("profileHeap() = %v, want %v", err, nil)
		t.FailNow()
	}
	if len(b) == 0 {
		t.Error("len of heap profile bytes= 0, want > 0")
	}
}
