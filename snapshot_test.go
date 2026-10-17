package main

import (
	"testing"
	"time"
)

func TestShouldSnapshot(t *testing.T) {
	for _, tc := range []struct {
		h    int
		want bool
	}{{2, true}, {1, false}, {23, false}} {
		got := shouldSnapshot(time.Date(2026, 1, 1, tc.h, 0, 0, 0, time.UTC))
		if got != tc.want {
			t.Fatalf("hour %d: got %v", tc.h, got)
		}
	}
}
