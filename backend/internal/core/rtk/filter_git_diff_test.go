package rtk

import (
	"strings"
	"testing"
)

func makeLongDiff() string {
	lines := []string{
		"diff --git a/foo.js b/foo.js",
		"index abc..def 100644",
		"--- a/foo.js",
		"+++ b/foo.js",
		"@@ -1,3 +1,200 @@",
	}
	for i := 0; i < 200; i++ {
		lines = append(lines, "+added line ")
	}
	return strings.Join(lines, "\n")
}

func TestGitDiff_TruncatesHunksAndKeepsHeader(t *testing.T) {
	input := makeLongDiff()
	out := gitDiffFilter(input)
	if !strings.Contains(out, "foo.js") {
		t.Errorf("expected output to contain file header 'foo.js', got: %q", out)
	}
	if !strings.Contains(out, "lines truncated") {
		t.Errorf("expected output to contain 'lines truncated', got: %q", out)
	}
	if len(out) >= len(input) {
		t.Errorf("expected truncation: out=%d input=%d", len(out), len(input))
	}
}

func TestGitDiff_ShortInputUnchangedShape(t *testing.T) {
	input := "diff --git a/x b/x\n@@ -1 +1 @@\n+a\n"
	out := gitDiffFilter(input)
	if !strings.Contains(out, "x") {
		t.Errorf("expected filename in output, got: %q", out)
	}
}
