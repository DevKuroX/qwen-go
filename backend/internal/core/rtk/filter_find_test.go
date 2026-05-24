package rtk

import (
	"fmt"
	"strings"
	"testing"
)

func makeFindOutput() string {
	lines := make([]string, 0, 60)
	for i := 0; i < 30; i++ {
		lines = append(lines, fmt.Sprintf("./src/a/%d.js", i))
	}
	for i := 0; i < 20; i++ {
		lines = append(lines, fmt.Sprintf("./src/b/%d.js", i))
	}
	for i := 0; i < 5; i++ {
		lines = append(lines, fmt.Sprintf("./top%d.md", i))
	}
	return strings.Join(lines, "\n")
}

func TestFind_GroupsByParentDir(t *testing.T) {
	input := makeFindOutput()
	out := findFilter(input)
	if !strings.Contains(out, "55 files in 3 dirs:") {
		t.Errorf("expected total header '55 files in 3 dirs:', got: %q", out)
	}
	if !strings.Contains(out, "./src/a/ (30):") {
		t.Errorf("expected ./src/a group, got: %q", out)
	}
	if !strings.Contains(out, "./src/b/ (20):") {
		t.Errorf("expected ./src/b group, got: %q", out)
	}
	if !strings.Contains(out, "./ (5):") {
		t.Errorf("expected root group ./ (5):, got: %q", out)
	}
	if len(out) >= len(input) {
		t.Errorf("expected compression: out=%d input=%d", len(out), len(input))
	}
}
