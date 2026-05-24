package rtk

import (
	"strings"
	"testing"
)

func TestDedupLog_CollapsesConsecutiveDuplicates(t *testing.T) {
	parts := []string{}
	for i := 0; i < 20; i++ {
		parts = append(parts, "repeated log line A")
	}
	parts = append(parts, "unique")
	for i := 0; i < 10; i++ {
		parts = append(parts, "another dup")
	}
	input := strings.Join(parts, "\n")
	out := dedupLogFilter(input)
	if !strings.Contains(out, "repeated log line A") {
		t.Errorf("expected original line preserved, got: %q", out)
	}
	if !strings.Contains(out, "duplicate lines") {
		t.Errorf("expected dedup summary, got: %q", out)
	}
	if len(out) >= len(input) {
		t.Errorf("expected compression: out=%d input=%d", len(out), len(input))
	}
}

func TestDedupLog_UniqueInputUnchanged(t *testing.T) {
	input := "alpha\nbeta\ngamma\ndelta\nepsilon"
	out := dedupLogFilter(input)
	for _, w := range []string{"alpha", "beta", "gamma", "delta", "epsilon"} {
		if !strings.Contains(out, w) {
			t.Errorf("expected unique line %q preserved, got: %q", w, out)
		}
	}
}
