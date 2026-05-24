package rtk

// Filter is a named text transform. Apply returns the compressed body or the
// original on no-op. Panics in Apply are caught by safeApply.
type Filter struct {
	Name  string
	Apply func(string) string
}

// safeApply mirrors 9router's applyFilter.safeApply — recovers from panics
// and falls back to the raw input, matching Rust's catch_unwind passthrough
// in the reference implementation.
func safeApply(f Filter, text string) (out string) {
	if f.Apply == nil {
		return text
	}
	defer func() {
		if r := recover(); r != nil {
			out = text
		}
	}()
	return f.Apply(text)
}
