package core

import (
	"strings"

	"github.com/qwenpi/qwenpi-go/internal/core/caveman"
	"github.com/qwenpi/qwenpi-go/internal/core/rtk"
)

// RequestSaverStats bundles the per-request RTK + Caveman outcomes for the
// request log. Mirrors what 9router stuffs into its log line.
type RequestSaverStats struct {
	RTK     *rtk.Stats
	Caveman caveman.Stats
}

// CompactionMode encodes the saver state into a single TEXT cell for the
// request_logs table. Examples: "rtk+caveman:full", "rtk", "caveman:ultra", "".
func (s RequestSaverStats) CompactionMode() string {
	parts := []string{}
	if s.RTK != nil && len(s.RTK.Hits) > 0 {
		parts = append(parts, "rtk")
	}
	if s.Caveman.Enabled && s.Caveman.Level != "" {
		parts = append(parts, "caveman:"+s.Caveman.Level)
	}
	return strings.Join(parts, "+")
}

// RTKReductionPct returns RTK-only byte reduction (Caveman adds bytes, so it's
// explicitly excluded).
func (s RequestSaverStats) RTKReductionPct() int {
	if s.RTK == nil {
		return 0
	}
	return s.RTK.ReductionPct()
}

// RTKFiltersCSV joins unique filter names that fired (preserving first-seen
// order) — stored in the new rtk_filters column.
func (s RequestSaverStats) RTKFiltersCSV() string {
	if s.RTK == nil {
		return ""
	}
	return strings.Join(s.RTK.FilterNames(), ",")
}
