package rtk

import (
	"fmt"
	"regexp"
	"strings"
	"testing"
)

func makeGrepOutput() string {
	lines := make([]string, 0, 50)
	for i := 1; i <= 40; i++ {
		lines = append(lines, fmt.Sprintf("src/foo.js:%d:const x%d = \"some value here with padding text padding text\"", i, i))
	}
	for i := 1; i <= 10; i++ {
		lines = append(lines, fmt.Sprintf("src/bar.js:%d:const y%d = \"another value here with padding padding padding\"", i, i))
	}
	return strings.Join(lines, "\n")
}

func TestGrep_GroupsByFileAndCapsPerFile(t *testing.T) {
	input := makeGrepOutput()
	out := grepFilter(input)
	if !strings.Contains(out, "50 matches in 2F:") {
		t.Errorf("expected total header '50 matches in 2F:', got: %q", out)
	}
	if !strings.Contains(out, "[file] src/foo.js (40):") {
		t.Errorf("expected foo.js group header, got: %q", out)
	}
	if !strings.Contains(out, "[file] src/bar.js (10):") {
		t.Errorf("expected bar.js group header, got: %q", out)
	}
	if ok, _ := regexp.MatchString(`\+\d+`, out); !ok {
		t.Errorf("expected per-file overflow marker (+N), got: %q", out)
	}
	if len(out) >= len(input) {
		t.Errorf("expected compression: out=%d input=%d", len(out), len(input))
	}
}

func TestGrep_NoMatchPassesThrough(t *testing.T) {
	input := "no colons here at all\njust prose\n"
	out := grepFilter(input)
	if out != input {
		t.Errorf("expected pass-through on no matches, got: %q", out)
	}
}
