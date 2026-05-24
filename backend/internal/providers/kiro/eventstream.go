package kiro

// AWS EventStream binary frame parser (vnd.amazon.eventstream).
// Ported from /home/ubuntu/ai_proxy/_ref/9router/open-sse/executors/kiro.js
// parseEventFrame. CRC bytes are present in the protocol but not validated —
// upstream framing is reliable in practice and adding CRC32-IEEE here just
// adds CPU for no observed reliability gain (matches the reference port).

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
)

// Frame is a single decoded EventStream message. Headers preserves all known
// keys (":event-type", ":content-type", ":message-type", etc.). Payload is
// the raw JSON-decoded body — most CodeWhisperer event payloads are objects,
// but tool-use frames can be arrays so we keep it as interface{}.
type Frame struct {
	Headers map[string]string
	Payload map[string]interface{}
}

// EventType returns the ":event-type" header, or "" if absent.
func (f *Frame) EventType() string {
	if f == nil {
		return ""
	}
	return f.Headers[":event-type"]
}

// errPartialFrame signals "not enough bytes yet" so the streaming reader can
// wait for more data without treating it as a parse failure.
var errPartialFrame = errors.New("eventstream: partial frame")

// ReadFrame pulls a single complete frame off the reader. Returns io.EOF
// cleanly when the stream is consumed at a frame boundary.
func ReadFrame(r io.Reader) (*Frame, error) {
	var preludeLen [4]byte
	if _, err := io.ReadFull(r, preludeLen[:]); err != nil {
		return nil, err
	}
	totalLen := binary.BigEndian.Uint32(preludeLen[:])
	if totalLen < 16 {
		return nil, errors.New("eventstream: malformed prelude (total_len < 16)")
	}

	// Read the rest of the message into a single buffer so we can hand the
	// already-known total_len bytes off to parseFrameBody without juggling
	// partial reads inside the header loop.
	rest := make([]byte, totalLen-4)
	if _, err := io.ReadFull(r, rest); err != nil {
		return nil, err
	}

	full := make([]byte, totalLen)
	copy(full[:4], preludeLen[:])
	copy(full[4:], rest)
	return parseFrameBody(full)
}

// parseFrameBody parses a complete frame already in memory. Exposed so the
// stream reader can buffer multiple frames out of a single Read() call.
func parseFrameBody(data []byte) (*Frame, error) {
	if len(data) < 16 {
		return nil, errPartialFrame
	}
	totalLen := binary.BigEndian.Uint32(data[0:4])
	if int(totalLen) != len(data) {
		return nil, errors.New("eventstream: declared total_len does not match buffer")
	}
	headersLen := binary.BigEndian.Uint32(data[4:8])
	// data[8:12] is prelude CRC, data[totalLen-4:] is message CRC — not validated.

	headerEnd := 12 + int(headersLen)
	if headerEnd > len(data)-4 {
		return nil, errors.New("eventstream: headers extend past payload region")
	}

	headers := make(map[string]string)
	off := 12
	for off < headerEnd {
		if off+1 > headerEnd {
			break
		}
		nameLen := int(data[off])
		off++
		if off+nameLen > headerEnd {
			break
		}
		name := string(data[off : off+nameLen])
		off += nameLen

		if off+1 > headerEnd {
			break
		}
		hType := data[off]
		off++

		// Type 7 = string (the only type CodeWhisperer event-stream uses for
		// the headers we care about). Bail on anything else rather than
		// guess at value-byte lengths.
		if hType != 7 {
			break
		}
		if off+2 > headerEnd {
			break
		}
		valueLen := int(binary.BigEndian.Uint16(data[off : off+2]))
		off += 2
		if off+valueLen > headerEnd {
			break
		}
		headers[name] = string(data[off : off+valueLen])
		off += valueLen
	}

	payloadStart := 12 + int(headersLen)
	payloadEnd := int(totalLen) - 4
	frame := &Frame{Headers: headers}
	if payloadEnd > payloadStart {
		raw := data[payloadStart:payloadEnd]
		// Some keep-alive frames carry no body — leave Payload nil.
		if len(raw) > 0 {
			var obj map[string]interface{}
			if err := json.Unmarshal(raw, &obj); err == nil {
				frame.Payload = obj
			}
			// Unmarshal failure is non-fatal: surface the frame with headers
			// only so the caller can decide based on event-type (e.g. an
			// empty messageStopEvent is meaningful even without a body).
		}
	}
	return frame, nil
}

// StreamParser is a stateful parser for HTTP response bodies. It buffers
// across reads and yields frames as they complete.
type StreamParser struct {
	buf []byte
}

// NewStreamParser returns a fresh parser ready to accept Feed() calls.
func NewStreamParser() *StreamParser { return &StreamParser{} }

// Feed appends bytes to the internal buffer and returns any frames that are
// now fully present. Partial trailing data stays buffered.
func (s *StreamParser) Feed(chunk []byte) ([]*Frame, error) {
	s.buf = append(s.buf, chunk...)
	var out []*Frame
	for len(s.buf) >= 4 {
		totalLen := binary.BigEndian.Uint32(s.buf[:4])
		if totalLen < 16 || int(totalLen) > 16*1024*1024 {
			// Sanity: 16 MB single frame is well past any real upstream
			// message. Drop the buffer to avoid pinning a malformed stream.
			return out, errors.New("eventstream: implausible frame length")
		}
		if int(totalLen) > len(s.buf) {
			break
		}
		frameBytes := s.buf[:totalLen]
		frame, err := parseFrameBody(frameBytes)
		if err != nil {
			if err == errPartialFrame {
				break
			}
			// Discard the bad frame and continue — better than stalling the
			// stream on one corrupt message.
			s.buf = s.buf[totalLen:]
			continue
		}
		s.buf = s.buf[totalLen:]
		if frame != nil {
			out = append(out, frame)
		}
	}
	return out, nil
}
