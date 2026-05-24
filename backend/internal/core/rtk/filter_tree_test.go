package rtk

import (
	"strings"
	"testing"
)

func TestTree_RemovesSummaryKeepsStructure(t *testing.T) {
	input := ".\n├── src\n│   └── main.rs\n└── Cargo.toml\n\n2 directories, 3 files\n"
	out := treeFilter(input)
	if strings.Contains(out, "directories") {
		t.Errorf("expected summary line dropped, got: %q", out)
	}
	if !strings.Contains(out, "├──") {
		t.Errorf("expected tree glyphs preserved, got: %q", out)
	}
	if !strings.Contains(out, "main.rs") {
		t.Errorf("expected file preserved, got: %q", out)
	}
}
