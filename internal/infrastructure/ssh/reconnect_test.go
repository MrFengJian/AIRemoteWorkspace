package ssh

import (
	"testing"
	"time"
)

func TestReconnectBackoff(t *testing.T) {
	cases := []struct {
		attempt int
		want    time.Duration
	}{
		{1, 2 * time.Second},
		{2, 4 * time.Second},
		{3, 8 * time.Second},
		{4, 16 * time.Second},
		{5, 30 * time.Second},  // capped
		{10, 30 * time.Second}, // still capped
	}
	for _, c := range cases {
		if got := reconnectBackoff(c.attempt); got != c.want {
			t.Fatalf("reconnectBackoff(%d) = %v, want %v", c.attempt, got, c.want)
		}
	}
}
