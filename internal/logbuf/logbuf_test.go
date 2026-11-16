package logbuf

import (
	"fmt"
	"strings"
	"testing"
)

func TestKeepsNewestLinesAndFilters(t *testing.T) {
	b := New(3)
	for i := 1; i <= 5; i++ {
		fmt.Fprintf(b, "undermynd line %d job ab%d\n", i, i)
	}
	got := b.Lines("", 10)
	if len(got) != 3 || !strings.Contains(got[0], "line 3") || !strings.Contains(got[2], "line 5") {
		t.Fatalf("got %v", got)
	}
	if f := b.Lines("AB4 JOB", 10); len(f) != 1 || !strings.Contains(f[0], "line 4") {
		t.Fatalf("filter: %v", f)
	}
	if n := b.Lines("", 2); len(n) != 2 || !strings.Contains(n[1], "line 5") {
		t.Fatalf("limit: %v", n)
	}
}

func TestPartialLinesJoinAndSetupKeyIsDropped(t *testing.T) {
	b := New(10)
	_, _ = b.Write([]byte("half a li"))
	_, _ = b.Write([]byte("ne\nno admin account yet. Open /admin and use setup key: SECRET\n"))
	got := b.Lines("", 10)
	if len(got) != 1 || got[0] != "half a line" {
		t.Fatalf("got %v", got)
	}
}
