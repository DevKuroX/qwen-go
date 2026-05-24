package rtk

import (
	"strings"
	"testing"
)

func TestLs_StripsPermsKeepsNameAndSize(t *testing.T) {
	input := strings.Join([]string{
		"total 48",
		"drwxr-xr-x  2 user staff   64 Jan  1 12:00 .",
		"drwxr-xr-x  2 user staff   64 Jan  1 12:00 ..",
		"drwxr-xr-x  2 user staff   64 Jan  1 12:00 src",
		"-rw-r--r--  1 user staff 1234 Jan  1 12:00 Cargo.toml",
		"-rw-r--r--  1 user staff 5678 Jan  1 12:00 README.md",
	}, "\n")
	out := lsFilter(input)
	for _, w := range []string{"src/", "Cargo.toml", "1.2K", "5.5K"} {
		if !strings.Contains(out, w) {
			t.Errorf("expected %q in output, got: %q", w, out)
		}
	}
	if strings.Contains(out, "drwx") {
		t.Errorf("expected perms stripped, got: %q", out)
	}
	if !strings.Contains(out, "Summary: 2 files, 1 dirs") {
		t.Errorf("expected file/dir summary, got: %q", out)
	}
}

func TestLs_FiltersNoiseDirs(t *testing.T) {
	input := strings.Join([]string{
		"total 8",
		"drwxr-xr-x  2 user staff 64 Jan  1 12:00 node_modules",
		"drwxr-xr-x  2 user staff 64 Jan  1 12:00 .git",
		"drwxr-xr-x  2 user staff 64 Jan  1 12:00 src",
		"-rw-r--r--  1 user staff 100 Jan  1 12:00 main.js",
	}, "\n")
	out := lsFilter(input)
	if strings.Contains(out, "node_modules") {
		t.Errorf("expected node_modules filtered, got: %q", out)
	}
	if strings.Contains(out, ".git") {
		t.Errorf("expected .git filtered, got: %q", out)
	}
	if !strings.Contains(out, "src/") || !strings.Contains(out, "main.js") {
		t.Errorf("expected useful entries kept, got: %q", out)
	}
}
