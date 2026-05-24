package rtk

// Hit records one compression event. Saved is bytes removed (always >= 0
// because compressText guards against never-grow).
type Hit struct {
	Shape  string
	Filter string
	Saved  int
}

// Stats accumulates byte deltas + per-hit details across a single
// compressMessages walk. Mirrors 9router stats shape used by formatRtkLog.
type Stats struct {
	BytesBefore int
	BytesAfter  int
	Hits        []Hit
}

// FilterNames returns the unique filter names that fired, preserving
// first-seen order (matches Array.from(new Set(...)) ordering in JS).
func (s *Stats) FilterNames() []string {
	if s == nil || len(s.Hits) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(s.Hits))
	out := make([]string, 0, len(s.Hits))
	for _, h := range s.Hits {
		if _, ok := seen[h.Filter]; ok {
			continue
		}
		seen[h.Filter] = struct{}{}
		out = append(out, h.Filter)
	}
	return out
}

// ReductionPct is integer-rounded percent of bytes removed. 0 if nothing
// fired.
func (s *Stats) ReductionPct() int {
	if s == nil || s.BytesBefore <= 0 {
		return 0
	}
	saved := s.BytesBefore - s.BytesAfter
	if saved <= 0 {
		return 0
	}
	return int(float64(saved) * 100 / float64(s.BytesBefore))
}
