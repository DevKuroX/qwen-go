// Package rtk ports 9router's tool-output compression pipeline
// (open-sse/rtk/). All constants mirror the JS reference verbatim.
package rtk

const (
	RawCap          = 10 * 1024 * 1024 // 10 MiB; skip huge blobs
	MinCompressSize = 500              // skip tiny blobs
	DetectWindow    = 1024             // autodetect peeks first N chars

	GitDiffHunkMaxLines = 100 // per-hunk line cap
	GitDiffContextKeep  = 3   // context lines around changes
	DedupLineMax        = 2000

	GrepPerFileMax   = 10
	FindPerDirMax    = 10
	FindTotalDirMax  = 20
	StatusMaxFiles   = 10
	StatusMaxUntrack = 10

	LsExtSummaryTop = 5
	TreeMaxLines    = 200

	SearchListPerDirMax   = 10
	SearchListTotalDirMax = 20

	SmartTruncateHead     = 120
	SmartTruncateTail     = 60
	SmartTruncateMinLines = 250

	ReadNumberedMinHitRatio = 0.7
)

// LsNoiseDirs are skipped from `ls -la` output so LLM context isn't
// polluted with node_modules / .git etc.
var LsNoiseDirs = map[string]struct{}{
	"node_modules": {}, ".git": {}, "target": {}, "__pycache__": {},
	".next": {}, "dist": {}, "build": {}, ".venv": {}, "venv": {},
	".cache": {}, ".idea": {}, ".vscode": {}, ".DS_Store": {},
}

// Filter name strings — kept in sync with 9router constants.js FILTERS.
const (
	NameGitDiff       = "git-diff"
	NameGitStatus     = "git-status"
	NameGitLog        = "git-log"
	NameGrep          = "grep"
	NameFind          = "find"
	NameLs            = "ls"
	NameTree          = "tree"
	NameDedupLog      = "dedup-log"
	NameSmartTruncate = "smart-truncate"
	NameReadNumbered  = "read-numbered"
	NameSearchList    = "search-list"
)
