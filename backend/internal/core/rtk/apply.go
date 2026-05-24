package rtk

// RawBody is the loose JSON shape the apply layer walks, matching 9router's
// runtime body inspection. Callers should decode with json.Decoder.UseNumber()
// so numeric IDs round-trip without precision loss when the body is
// re-serialised after mutation.
type RawBody = map[string]any

// CompressOpenAIChat walks body.messages[] for OpenAI chat-completions shape.
// Mutates in place; returns stats (never nil — empty if nothing compressed).
func CompressOpenAIChat(body RawBody) *Stats {
	stats := &Stats{}
	if body == nil {
		return stats
	}
	walkMessages(body, "messages", stats)
	return stats
}

// CompressOpenAIResponses walks body.input[] for the Responses API shape.
// Handles both function_call_output items and OpenAI tool messages.
func CompressOpenAIResponses(body RawBody) *Stats {
	stats := &Stats{}
	if body == nil {
		return stats
	}
	walkMessages(body, "input", stats)
	return stats
}

// CompressAnthropic walks body.messages[] for Claude /v1/messages shape — the
// tool_result blocks live inside content[] arrays with is_error skip.
func CompressAnthropic(body RawBody) *Stats {
	stats := &Stats{}
	if body == nil {
		return stats
	}
	walkMessages(body, "messages", stats)
	return stats
}

// CompressGemini is a near no-op: Gemini's request shape has no native
// tool_result blocks in body.contents[]. Kept for symmetry and future Gemini
// function-response support.
func CompressGemini(body RawBody) *Stats {
	return &Stats{}
}

// walkMessages mirrors 9router compressMessages: peek items[] under arrKey,
// dispatch on shape (function_call_output, role:tool string/array, tool_result
// blocks), call compressText in place.
func walkMessages(body RawBody, arrKey string, stats *Stats) {
	raw, ok := body[arrKey]
	if !ok {
		return
	}
	items, ok := raw.([]any)
	if !ok {
		return
	}
	for _, item := range items {
		msg, ok := item.(map[string]any)
		if !ok {
			continue
		}

		// Shape 4: Responses function_call_output
		if t, _ := msg["type"].(string); t == "function_call_output" {
			compressOutputField(msg, "output", stats,
				"openai-responses-string",
				"openai-responses-array",
				"input_text")
			continue
		}

		role, _ := msg["role"].(string)

		// Shape 1: OpenAI tool message with string content
		if role == "tool" {
			switch c := msg["content"].(type) {
			case string:
				msg["content"] = compressText(c, stats, "openai-tool")
				continue
			case []any:
				compressTextParts(c, stats, "openai-tool-array", "text")
				continue
			}
		}

		// Shape 2/3: Claude blocks array — tool_result entries
		blocks, ok := msg["content"].([]any)
		if !ok {
			continue
		}
		for _, b := range blocks {
			block, ok := b.(map[string]any)
			if !ok {
				continue
			}
			if t, _ := block["type"].(string); t != "tool_result" {
				continue
			}
			if isErr, _ := block["is_error"].(bool); isErr {
				continue
			}

			switch c := block["content"].(type) {
			case string:
				block["content"] = compressText(c, stats, "claude-string")
			case []any:
				compressTextParts(c, stats, "claude-array", "text")
			}
		}
	}
}

// compressOutputField — function_call_output uses "output" with "input_text"
// parts, while every other shape uses "content" + "text" parts. Same body
// otherwise.
func compressOutputField(msg map[string]any, key string, stats *Stats, strShape, arrShape, partType string) {
	switch v := msg[key].(type) {
	case string:
		msg[key] = compressText(v, stats, strShape)
	case []any:
		compressTextParts(v, stats, arrShape, partType)
	}
}

// compressTextParts walks a slice of {type, text} parts (or {type:"input_text", text}
// for Responses) and rewrites the text field in place.
func compressTextParts(parts []any, stats *Stats, shape, wantType string) {
	for _, p := range parts {
		part, ok := p.(map[string]any)
		if !ok {
			continue
		}
		if t, _ := part["type"].(string); t != wantType {
			continue
		}
		text, ok := part["text"].(string)
		if !ok {
			continue
		}
		part["text"] = compressText(text, stats, shape)
	}
}
