package rtk

import (
	"fmt"
	"strings"
	"testing"
)

func TestReadNumbered_CompactsLongDump(t *testing.T) {
	lines := make([]string, 0, 400)
	for i := 1; i <= 400; i++ {
		lines = append(lines, fmt.Sprintf("  %d|content %d", i, i))
	}
	input := strings.Join(lines, "\n")
	out := readNumberedFilter(input)
	if !strings.Contains(out, "1|content 1") {
		t.Errorf("expected first line preserved, got first 200 chars: %q", out[:min(200, len(out))])
	}
	if !strings.Contains(out, "400|content 400") {
		t.Errorf("expected last line preserved, got: tail %q", out[max(0, len(out)-200):])
	}
	if !strings.Contains(out, "lines truncated") {
		t.Errorf("expected truncation marker, got: %q", out)
	}
	if len(out) >= len(input) {
		t.Errorf("expected compression: out=%d input=%d", len(out), len(input))
	}
}
