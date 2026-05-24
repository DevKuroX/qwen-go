package rtk

import (
	"fmt"
	"strings"
	"testing"
)

func TestSmartTruncate_KeepsHeadAndTailDropsMiddle(t *testing.T) {
	lines := make([]string, 0, 400)
	for i := 0; i < 400; i++ {
		lines = append(lines, fmt.Sprintf("line %d", i))
	}
	input := strings.Join(lines, "\n")
	out := smartTruncateFilter(input)
	if !strings.Contains(out, "line 0") {
		t.Errorf("expected head preserved, got first 200 chars: %q", out[:min(200, len(out))])
	}
	if !strings.Contains(out, "line 399") {
		t.Errorf("expected tail preserved, got last 200 chars: %q", out[max(0, len(out)-200):])
	}
	if !strings.Contains(out, "lines truncated") {
		t.Errorf("expected truncation marker, got: %q", out)
	}
	if len(out) >= len(input) {
		t.Errorf("expected truncation: out=%d input=%d", len(out), len(input))
	}
}

func TestSmartTruncate_SmallInputPassesThrough(t *testing.T) {
	lines := make([]string, 0, 10)
	for i := 0; i < 10; i++ {
		lines = append(lines, fmt.Sprintf("line %d", i))
	}
	input := strings.Join(lines, "\n")
	out := smartTruncateFilter(input)
	if out != input {
		t.Errorf("expected pass-through on small input, got: %q", out)
	}
}

