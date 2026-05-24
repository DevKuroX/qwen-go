package rtk

import (
	"strings"
	"testing"
)

func TestAutoDetect_PriorityChain(t *testing.T) {
	// smart-truncate is the last-resort fallback: many lines but very few
	// non-empty ones (<5) so dedup-log doesn't catch first. Matches 9router's
	// autodetect.js fallback chain.
	bigSparse := func() string {
		lines := make([]string, 0, SmartTruncateMinLines+10)
		for i := 0; i < SmartTruncateMinLines+10; i++ {
			if i == 0 || i == 100 || i == 200 {
				lines = append(lines, "rare content")
			} else {
				lines = append(lines, "")
			}
		}
		return strings.Join(lines, "\n")
	}()

	cases := []struct {
		name string
		in   string
		want string
	}{
		{"git-diff via diff --git header", "diff --git a/x b/x\n@@ -1 +1 @@\n+a", NameGitDiff},
		{"git-diff via @@ hunk only", "@@ -10,5 +10,5 @@\n-old\n+new", NameGitDiff},
		{"git-status via On branch", "On branch main\n  modified:   x.js\n", NameGitStatus},
		{"grep via file:lineno:content", "a.js:1:hello\nb.js:2:world\nc.js:3:foo", NameGrep},
		{"find via dot-path triple", "./a/b.js\n./a/c.js\n./a/d.js", NameFind},
		{"tree via box glyphs", ".\n├── src\n│   └── main.rs\n└── Cargo.toml\n", NameTree},
		{"ls via total + perms rows", strings.Join([]string{
			"total 48",
			"drwxr-xr-x  2 user staff   64 Jan  1 12:00 src",
			"-rw-r--r--  1 user staff 1234 Jan  1 12:00 main.js",
			"-rw-r--r--  1 user staff 5678 Jan  1 12:00 README.md",
		}, "\n"), NameLs},
		{"search-list via Cursor header", "Result of search in '/x' (total 3 files):\n- a/b.js\n- a/c.js\n- a/d.js", NameSearchList},
		{"dedup-log fallback for prose", "line1\nline2\nline3\nline4\nline5\nline6\n", NameDedupLog},
		{"smart-truncate fallback for sparse long input", bigSparse, NameSmartTruncate},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := AutoDetect(c.in)
			if got.Name != c.want {
				t.Errorf("AutoDetect()=%q want=%q\ninput head: %q", got.Name, c.want, c.in[:min(len(c.in), 120)])
			}
		})
	}
}

func TestAutoDetect_EmptyAndTinyInputs(t *testing.T) {
	cases := []string{
		"",
		"hi",
		"single line of normal text",
	}
	for _, in := range cases {
		got := AutoDetect(in)
		if got.Apply != nil {
			t.Errorf("AutoDetect(%q).Name=%q want zero-Filter (nil Apply)", in, got.Name)
		}
	}
}
