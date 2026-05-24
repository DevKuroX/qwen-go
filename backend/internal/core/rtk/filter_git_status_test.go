package rtk

import (
	"regexp"
	"strings"
	"testing"
)

func makeGitStatus() string {
	return strings.Join([]string{
		"On branch main",
		"Your branch is up to date with 'origin/main'.",
		"",
		"Changes not staged for commit:",
		"  (use \"git add <file>...\" to update what will be committed)",
		"\tmodified:   src/a.js",
		"\tmodified:   src/b.js",
		"\tnew file:   src/c.js",
		"\tdeleted:    src/old.js",
		"",
		"Untracked files:",
		"\tnotes.txt",
		"",
		"no changes added to commit",
	}, "\n")
}

func TestGitStatus_GroupsAndCompactsRustFormat(t *testing.T) {
	input := makeGitStatus()
	out := gitStatusFilter(input)
	if !strings.Contains(out, "* main") {
		t.Errorf("expected branch marker '* main' in output, got: %q", out)
	}
	if ok, _ := regexp.MatchString(`~ Modified: \d+ files?`, out); !ok {
		t.Errorf("expected modified summary line, got: %q", out)
	}
	if !strings.Contains(out, "src/a.js") {
		t.Errorf("expected modified file listed, got: %q", out)
	}
	if len(out) >= len(input) {
		t.Errorf("expected compression: out=%d input=%d", len(out), len(input))
	}
}

func TestGitStatus_CleanTreeReturnsCleanMessage(t *testing.T) {
	input := "On branch main\nnothing to commit, working tree clean\n"
	out := gitStatusFilter(input)
	if !strings.Contains(strings.ToLower(out), "clean") {
		t.Errorf("expected clean-tree message, got: %q", out)
	}
}
