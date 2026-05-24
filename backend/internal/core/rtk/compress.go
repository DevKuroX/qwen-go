package rtk

// compressText is the core text mutation step. Mirrors 9router index.js
// compressText: guard with MIN/RAW caps, autodetect, safeApply, then enforce
// never-empty / never-grow. Always updates Stats.
func compressText(text string, stats *Stats, shape string) string {
	bytesIn := len(text)
	stats.BytesBefore += bytesIn

	if bytesIn < MinCompressSize || bytesIn > RawCap {
		stats.BytesAfter += bytesIn
		return text
	}

	f := AutoDetect(text)
	if f.Apply == nil {
		stats.BytesAfter += bytesIn
		return text
	}

	out := safeApply(f, text)
	if len(out) == 0 || len(out) >= bytesIn {
		stats.BytesAfter += bytesIn
		return text
	}

	stats.BytesAfter += len(out)
	stats.Hits = append(stats.Hits, Hit{Shape: shape, Filter: f.Name, Saved: bytesIn - len(out)})
	return out
}
