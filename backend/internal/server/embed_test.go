package server

import "testing"

func TestCleanPathRemovesNextMetadata(t *testing.T) {
	if got := cleanPath("/foo/__next.data"); got != "" {
		t.Fatalf("cleanPath() = %q, want empty", got)
	}
	if got := cleanPath("/foo/index.html"); got != "/foo/index.html" {
		t.Fatalf("cleanPath() = %q, want unchanged", got)
	}
}
