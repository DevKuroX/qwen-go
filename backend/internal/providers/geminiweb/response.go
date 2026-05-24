package geminiweb

// Frame parser ported from /home/ubuntu/ai_proxy/backend/internal/geminiweb/
// response.go. Contains 8 known parser-bug fixes — DO NOT rewrite from
// scratch, modify in-place if Gemini's frame format changes.

import (
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"unicode/utf16"
)

// ParseStream reads the length-prefixed-frame body and emits parsed chunks
// on chunkChan. Closes the channel on return.
func ParseStream(reader io.Reader, chunkChan chan<- geminiResponse) error {
	defer close(chunkChan)
	buf := make([]byte, 32*1024)
	var sb strings.Builder
	for {
		n, err := reader.Read(buf)
		if n > 0 {
			sb.Write(buf[:n])
			content := sb.String()
			if strings.HasPrefix(content, ")]}'") {
				content = strings.TrimLeft(content[4:], " \t\r\n")
			}
			for {
				frame, remaining, ok := parseNextFrame(content)
				if !ok {
					break
				}
				content = remaining
				chunk, chunkErr := extractResponseFromFrame(frame)
				if chunkErr == nil && chunk != nil {
					chunkChan <- *chunk
					if chunk.Done {
						// Upstream never sends a natural EOF after "e";
						// returning here lets the channel close.
						return nil
					}
				}
			}
			sb.Reset()
			sb.WriteString(content)
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// parseNextFrame extracts one length-prefixed JSON frame from buffer. Length
// is in UTF-16 code units (matches JavaScript String.length on upstream), and
// the leading newline counts in the length — that's bug-fix #2.
func parseNextFrame(buffer string) (string, string, bool) {
	buffer = strings.TrimLeft(buffer, " \t\r\n")
	if buffer == "" {
		return "", buffer, false
	}

	nl := strings.IndexByte(buffer, '\n')
	if nl < 0 {
		return "", buffer, false
	}

	lengthStr := strings.TrimSpace(buffer[:nl])
	length, err := strconv.Atoi(lengthStr)
	if err != nil || length <= 0 {
		return "", buffer, false
	}

	frameStart := nl
	if frameStart >= len(buffer) {
		return "", buffer, false
	}

	payload := buffer[frameStart:]
	codeUnits := 0
	bytesConsumed := 0
	for i, r := range payload {
		codeUnits++
		if r >= 0x10000 {
			codeUnits++
		}
		if codeUnits >= length {
			bytesConsumed = i + len(string(r))
			break
		}
	}

	if codeUnits < length {
		return "", buffer, false
	}

	frame := payload[:bytesConsumed]
	if len(frame) > 0 && frame[0] == '\n' {
		frame = frame[1:]
	}
	remaining := payload[bytesConsumed:]
	return frame, remaining, true
}

func extractResponseFromText(body []byte) (*geminiResponse, error) {
	raw := string(body)
	if strings.HasPrefix(raw, ")]}'") {
		raw = strings.TrimLeft(raw[4:], " \t\r\n")
	}

	remaining := raw
	var lastResult *geminiResponse
	for {
		frame, rem, ok := parseNextFrame(remaining)
		if !ok {
			break
		}
		remaining = rem
		chunk, err := extractResponseFromFrame(frame)
		if err == nil && chunk != nil && chunk.Text != "" {
			lastResult = chunk
		}
	}
	if lastResult != nil {
		return lastResult, nil
	}

	var parsed []interface{}
	if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &parsed); err == nil {
		result := &geminiResponse{}
		for _, item := range parsed {
			if tuple, ok := item.([]interface{}); ok && len(tuple) >= 1 {
				code, _ := tuple[0].(string)
				switch code {
				case "wra", "wrb.fr":
					if len(tuple) >= 3 {
						if payloadStr, ok := tuple[2].(string); ok && payloadStr != "" {
							parseInnerPayload(payloadStr, result)
						}
					}
				case "di":
					if len(tuple) > 1 {
						if v, ok := tuple[1].(float64); ok && v >= 2 {
							result.Done = true
						}
					}
				}
			}
		}
		if result.Text != "" {
			return result, nil
		}
	}

	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var parsedItem interface{}
		if err := json.Unmarshal([]byte(line), &parsedItem); err == nil {
			if tuple, ok := parsedItem.([]interface{}); ok && len(tuple) >= 1 {
				code, _ := tuple[0].(string)
				if code == "wra" || code == "wrb.fr" {
					if len(tuple) >= 3 {
						if payloadStr, ok := tuple[2].(string); ok && payloadStr != "" {
							result := &geminiResponse{}
							parseInnerPayload(payloadStr, result)
							if result.Text != "" {
								return result, nil
							}
						}
					}
				}
			}
		}
	}

	sample := raw
	if len(sample) > 200 {
		sample = sample[:200]
	}
	return nil, fmt.Errorf("no valid response found (body: %s)", sample)
}

func extractResponseFromFrame(frame string) (*geminiResponse, error) {
	frame = strings.TrimSpace(frame)
	if frame == "" {
		return nil, fmt.Errorf("empty frame")
	}

	var tuples []interface{}
	if err := json.Unmarshal([]byte(frame), &tuples); err != nil {
		return nil, fmt.Errorf("json parse error: %w", err)
	}

	result := &geminiResponse{}

	for _, t := range tuples {
		tuple, ok := t.([]interface{})
		if !ok || len(tuple) < 1 {
			continue
		}

		code, _ := tuple[0].(string)

		switch code {
		case "wra", "wrb.fr":
			payloadStr, ok := tuple[2].(string)
			if !ok || payloadStr == "" {
				continue
			}
			parseInnerPayload(payloadStr, result)

		case "di":
			// Bug-fix #5: real values are large (e.g. 4935), not exactly 2.
			if len(tuple) > 1 {
				if v, ok := tuple[1].(float64); ok && v >= 2 {
					result.Done = true
				}
			}

		case "e":
			// Bug-fix #6: end-of-stream sentinel — must set Done and return.
			result.Done = true
		}
	}

	return result, nil
}

func parseInnerPayload(payloadStr string, result *geminiResponse) {
	var payload []interface{}
	if err := json.Unmarshal([]byte(payloadStr), &payload); err != nil {
		return
	}
	if len(payload) < 5 {
		return
	}

	candidates, ok := payload[4].([]interface{})
	if !ok {
		return
	}
	for _, cand := range candidates {
		candData, ok := cand.([]interface{})
		if !ok || len(candData) < 2 {
			continue
		}
		if content, ok := candData[1].([]interface{}); ok && len(content) > 0 {
			if text, ok := content[0].(string); ok && text != "" {
				result.Text = text
			}
		}
		// Image harvesting — generated/edited images live in candidate[12].
		// Two slots per HanaokaYuzu/Gemini-API client.py:1443-1446:
		//   [12][7][0]      → text-to-image (plain generation)
		//   [12][0]["8"][0] → image-to-image (edit)
		if len(candData) > 12 {
			extractImagesFromSlot(candData[12], result)
		}
	}
}

// extractImagesFromSlot walks the [12]th candidate slot for both the plain
// and image-edit listings. The structure per image entry is:
//   item[0][3][3] → image URL
//   item[0][3][2] → alt text
func extractImagesFromSlot(slot interface{}, result *geminiResponse) {
	arr, ok := slot.([]interface{})
	if !ok {
		return
	}

	// Plain generation: [12][7][0] is a list of image entries.
	if len(arr) > 7 {
		if seven, ok := arr[7].([]interface{}); ok && len(seven) > 0 {
			if list, ok := seven[0].([]interface{}); ok {
				for _, item := range list {
					if img := extractImageEntry(item); img != nil {
						result.Images = append(result.Images, *img)
					}
				}
			}
		}
	}

	// Image-to-image edit: [12][0] is sometimes a dict-ish [["8", […]]]
	// shape. We accept either a map or the JSON-array fallback.
	if len(arr) > 0 {
		switch zero := arr[0].(type) {
		case map[string]interface{}:
			if eight, ok := zero["8"].([]interface{}); ok && len(eight) > 0 {
				if list, ok := eight[0].([]interface{}); ok {
					for _, item := range list {
						if img := extractImageEntry(item); img != nil {
							result.Images = append(result.Images, *img)
						}
					}
				}
			}
		}
	}
}

func extractImageEntry(item interface{}) *generatedImage {
	tuple, ok := item.([]interface{})
	if !ok || len(tuple) < 1 {
		return nil
	}
	zero, ok := tuple[0].([]interface{})
	if !ok || len(zero) < 4 {
		return nil
	}
	three, ok := zero[3].([]interface{})
	if !ok || len(three) < 4 {
		return nil
	}
	url, _ := three[3].(string)
	if url == "" {
		return nil
	}
	alt, _ := three[2].(string)
	return &generatedImage{URL: url, Alt: alt}
}

func parseNonStreamingResponse(body []byte) (*geminiResponse, error) {
	return extractResponseFromText(body)
}

// preserved for compatibility with downstream callers that import utf16
var _ = utf16.Encode
