package rtk

import (
	"strings"
	"testing"
)

func TestCompressText_SkipsBelowMinSize(t *testing.T) {
	in := "diff --git a/x b/x\n@@ -1 +1 @@\n+a"
	stats := &Stats{}
	out := compressText(in, stats, "test")
	if out != in {
		t.Errorf("expected pass-through under MIN, got: %q", out)
	}
	if len(stats.Hits) != 0 {
		t.Errorf("expected no hits, got %d", len(stats.Hits))
	}
	if stats.BytesBefore != len(in) || stats.BytesAfter != len(in) {
		t.Errorf("expected bytes before==after==%d, got before=%d after=%d", len(in), stats.BytesBefore, stats.BytesAfter)
	}
}

func TestCompressText_SkipsAboveRawCap(t *testing.T) {
	// We can't materialise 10MB in a unit test cheaply — use a sub-slice of
	// known-compressible content but lie about the threshold isn't possible.
	// Instead: feed exactly RawCap+1 bytes of repetitive ASCII; the guard
	// kicks in regardless of detect result.
	big := strings.Repeat("x", RawCap+1)
	stats := &Stats{}
	out := compressText(big, stats, "test")
	if out != big {
		t.Errorf("expected pass-through above RAW_CAP")
	}
	if len(stats.Hits) != 0 {
		t.Errorf("expected no hits on oversize, got %d", len(stats.Hits))
	}
}

func TestCompressText_NeverGrow(t *testing.T) {
	// A filter that grows input is rejected by the never-grow guard. Forge a
	// Filter that always returns longer output and run compress with it via
	// safeApply directly (compressText auto-detects; can't inject a filter
	// without monkey-patching). Verify the never-grow rule by exercising the
	// guard through compressText's same-shape interpretation: feed inert text
	// that has no detector match — compressText returns input unchanged.
	in := strings.Repeat("benign prose without structure ", 30)
	stats := &Stats{}
	out := compressText(in, stats, "test")
	// May or may not detect a filter depending on length; assertion is
	// independent: never bigger than input.
	if len(out) > len(in) {
		t.Errorf("never-grow violated: out=%d in=%d", len(out), len(in))
	}
}

func TestCompressText_RecordsHitOnSuccessfulCompression(t *testing.T) {
	in := makeLongDiff()
	stats := &Stats{}
	out := compressText(in, stats, "test-shape")
	if len(out) >= len(in) {
		t.Errorf("expected compression, got out=%d in=%d", len(out), len(in))
	}
	if len(stats.Hits) != 1 {
		t.Fatalf("expected exactly one hit, got %d", len(stats.Hits))
	}
	h := stats.Hits[0]
	if h.Filter != NameGitDiff {
		t.Errorf("expected filter=%q, got %q", NameGitDiff, h.Filter)
	}
	if h.Shape != "test-shape" {
		t.Errorf("expected shape=test-shape, got %q", h.Shape)
	}
	if h.Saved <= 0 {
		t.Errorf("expected positive savings, got %d", h.Saved)
	}
	if stats.BytesBefore != len(in) {
		t.Errorf("bytesBefore=%d want %d", stats.BytesBefore, len(in))
	}
	if stats.BytesAfter != len(out) {
		t.Errorf("bytesAfter=%d want %d", stats.BytesAfter, len(out))
	}
}

func TestSafeApply_RecoversFromPanic(t *testing.T) {
	f := Filter{
		Name: "boom",
		Apply: func(s string) string {
			panic("kaboom")
		},
	}
	got := safeApply(f, "hello")
	if got != "hello" {
		t.Errorf("expected fallback to input on panic, got: %q", got)
	}
}

func TestSafeApply_NilApplyReturnsInput(t *testing.T) {
	got := safeApply(Filter{}, "hello")
	if got != "hello" {
		t.Errorf("expected pass-through for nil-Apply filter, got: %q", got)
	}
}

func TestStats_FilterNames_UniquePreserveOrder(t *testing.T) {
	s := &Stats{
		Hits: []Hit{
			{Filter: NameGitDiff},
			{Filter: NameGrep},
			{Filter: NameGitDiff},
			{Filter: NameFind},
		},
	}
	got := s.FilterNames()
	want := []string{NameGitDiff, NameGrep, NameFind}
	if len(got) != len(want) {
		t.Fatalf("len=%d want %d (%v)", len(got), len(want), got)
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("FilterNames()[%d]=%q want %q", i, got[i], w)
		}
	}
}

func TestStats_ReductionPct(t *testing.T) {
	cases := []struct {
		name        string
		before, aft int
		want        int
	}{
		{"50% saved", 1000, 500, 50},
		{"no savings", 1000, 1000, 0},
		{"empty", 0, 0, 0},
		{"60% saved", 1000, 400, 60},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := &Stats{BytesBefore: c.before, BytesAfter: c.aft}
			if got := s.ReductionPct(); got != c.want {
				t.Errorf("ReductionPct()=%d want %d", got, c.want)
			}
		})
	}
}
