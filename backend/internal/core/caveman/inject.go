package caveman

// RawBody is the loose JSON shape the injectors mutate. Mirrors rtk.RawBody so
// handlers can share a single map across both pipelines.
type RawBody = map[string]any

// InjectOpenAIChat handles body.messages[] (chat-completions). Append to first
// system/developer message, or unshift a fresh system message.
func InjectOpenAIChat(body RawBody, level string) Stats {
	prompt := Prompt(level)
	if body == nil || prompt == "" {
		return Stats{}
	}
	injectMessagesSystem(body, prompt)
	return Stats{Enabled: true, Level: level}
}

// InjectOpenAIResponses handles body.instructions (string) and body.input[]
// (array). 9router checks instructions first.
func InjectOpenAIResponses(body RawBody, level string) Stats {
	prompt := Prompt(level)
	if body == nil || prompt == "" {
		return Stats{}
	}
	injectMessagesSystem(body, prompt)
	return Stats{Enabled: true, Level: level}
}

// InjectAnthropic handles body.system as string or array, preserving the last
// cache_control block — caveman is inserted BEFORE that block so it stays
// inside the cached prefix.
func InjectAnthropic(body RawBody, level string) Stats {
	prompt := Prompt(level)
	if body == nil || prompt == "" {
		return Stats{}
	}
	injectClaudeSystem(body, prompt)
	return Stats{Enabled: true, Level: level}
}

// InjectGemini handles body.systemInstruction / body.system_instruction under
// body.* or body.request.* (the Antigravity wrap).
func InjectGemini(body RawBody, level string) Stats {
	prompt := Prompt(level)
	if body == nil || prompt == "" {
		return Stats{}
	}
	injectGeminiSystem(body, prompt)
	return Stats{Enabled: true, Level: level}
}

func injectMessagesSystem(body RawBody, prompt string) {
	if instr, ok := body["instructions"].(string); ok {
		if instr != "" {
			body["instructions"] = instr + Sep + prompt
		} else {
			body["instructions"] = prompt
		}
		return
	}

	arr, key := pickMessagesArray(body)
	if arr == nil {
		return
	}

	for _, item := range arr {
		msg, ok := item.(map[string]any)
		if !ok {
			continue
		}
		role, _ := msg["role"].(string)
		if role == "system" || role == "developer" {
			appendToOpenAIMessage(msg, prompt)
			return
		}
	}

	// No system/developer message — unshift a fresh one.
	newMsg := map[string]any{"role": "system", "content": prompt}
	body[key] = append([]any{newMsg}, arr...)
}

func pickMessagesArray(body RawBody) ([]any, string) {
	if msgs, ok := body["messages"].([]any); ok {
		return msgs, "messages"
	}
	if input, ok := body["input"].([]any); ok {
		return input, "input"
	}
	return nil, ""
}

func appendToOpenAIMessage(msg map[string]any, prompt string) {
	switch c := msg["content"].(type) {
	case string:
		msg["content"] = c + Sep + prompt
	case []any:
		msg["content"] = append(c, map[string]any{"type": "input_text", "text": prompt})
	default:
		msg["content"] = prompt
	}
}

func injectClaudeSystem(body RawBody, prompt string) {
	switch sys := body["system"].(type) {
	case string:
		if sys != "" {
			body["system"] = sys + Sep + prompt
			return
		}
		body["system"] = prompt
	case []any:
		block := map[string]any{"type": "text", "text": prompt}
		lastCacheIdx := -1
		for i := len(sys) - 1; i >= 0; i-- {
			elem, ok := sys[i].(map[string]any)
			if !ok {
				continue
			}
			if _, has := elem["cache_control"]; has {
				lastCacheIdx = i
				break
			}
		}
		if lastCacheIdx >= 0 {
			out := make([]any, 0, len(sys)+1)
			out = append(out, sys[:lastCacheIdx]...)
			out = append(out, block)
			out = append(out, sys[lastCacheIdx:]...)
			body["system"] = out
		} else {
			body["system"] = append(sys, block)
		}
	default:
		body["system"] = prompt
	}
}

func injectGeminiSystem(body RawBody, prompt string) {
	target := body
	if req, ok := body["request"].(map[string]any); ok {
		target = req
	}
	key := "systemInstruction"
	if _, hasSnake := target["system_instruction"]; hasSnake {
		key = "system_instruction"
	}
	if sys, ok := target[key].(map[string]any); ok {
		if parts, ok := sys["parts"].([]any); ok {
			sys["parts"] = append(parts, map[string]any{"text": prompt})
			return
		}
	}
	target[key] = map[string]any{
		"parts": []any{map[string]any{"text": prompt}},
	}
}
