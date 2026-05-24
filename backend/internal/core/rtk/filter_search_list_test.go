package rtk

import (
	"fmt"
	"regexp"
	"strings"
	"testing"
)

func TestSearchList_GroupsByParentDir(t *testing.T) {
	paths := make([]string, 0, 40)
	for i := 0; i < 30; i++ {
		paths = append(paths, fmt.Sprintf("- src/a/f%d.js", i))
	}
	for i := 0; i < 10; i++ {
		paths = append(paths, fmt.Sprintf("- src/b/g%d.js", i))
	}
	header := "Result of search in '/Users/x' (total 40 files):"
	input := header + "\n" + strings.Join(paths, "\n")
	out := searchListFilter(input)

	if !strings.Contains(out, "Result of search in") {
		t.Errorf("expected header preserved, got: %q", out)
	}
	if !strings.Contains(out, "40 files in 2 dirs:") {
		t.Errorf("expected summary header '40 files in 2 dirs:', got: %q", out)
	}
	if !strings.Contains(out, "src/a/ (30):") {
		t.Errorf("expected src/a group, got: %q", out)
	}
	if !strings.Contains(out, "src/b/ (10):") {
		t.Errorf("expected src/b group, got: %q", out)
	}
	if ok, _ := regexp.MatchString(`\+\d+`, out); !ok {
		t.Errorf("expected overflow marker (+N), got: %q", out)
	}
	if len(out) >= len(input) {
		t.Errorf("expected compression: out=%d input=%d", len(out), len(input))
	}
}
