package caveman

import (
	"strings"
	"testing"
)

func TestPrompt_KnownLevelsAndUnknown(t *testing.T) {
	cases := []struct {
		level string
		want  string
	}{
		{LevelLite, Prompts[LevelLite]},
		{LevelFull, Prompts[LevelFull]},
		{LevelUltra, Prompts[LevelUltra]},
		{"", ""},
		{"bogus", ""},
	}
	for _, c := range cases {
		if got := Prompt(c.level); got != c.want {
			t.Errorf("Prompt(%q) mismatch", c.level)
		}
	}
}

func TestInjectOpenAIChat_AppendsToExistingSystemString(t *testing.T) {
	body := RawBody{
		"messages": []any{
			map[string]any{"role": "system", "content": "you are helpful"},
			map[string]any{"role": "user", "content": "hi"},
		},
	}
	stats := InjectOpenAIChat(body, LevelFull)
	if !stats.Enabled || stats.Level != LevelFull {
		t.Errorf("expected Enabled+Level=full, got %+v", stats)
	}
	got := body["messages"].([]any)[0].(map[string]any)["content"].(string)
	if !strings.HasPrefix(got, "you are helpful"+Sep) {
		t.Errorf("expected existing system kept as prefix, got: %q", got)
	}
	if !strings.HasSuffix(got, Prompts[LevelFull]) {
		t.Errorf("expected full prompt as suffix")
	}
}

func TestInjectOpenAIChat_UnshiftsWhenNoSystem(t *testing.T) {
	body := RawBody{
		"messages": []any{
			map[string]any{"role": "user", "content": "hi"},
		},
	}
	stats := InjectOpenAIChat(body, LevelLite)
	if !stats.Enabled {
		t.Fatalf("expected enabled")
	}
	msgs := body["messages"].([]any)
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages after unshift, got %d", len(msgs))
	}
	first := msgs[0].(map[string]any)
	if first["role"] != "system" || first["content"] != Prompts[LevelLite] {
		t.Errorf("expected unshifted system message with lite prompt, got %+v", first)
	}
}

func TestInjectOpenAIChat_AppendsToDeveloperRole(t *testing.T) {
	body := RawBody{
		"messages": []any{
			map[string]any{"role": "developer", "content": "rules here"},
			map[string]any{"role": "user", "content": "hi"},
		},
	}
	InjectOpenAIChat(body, LevelUltra)
	dev := body["messages"].([]any)[0].(map[string]any)
	if !strings.Contains(dev["content"].(string), Prompts[LevelUltra]) {
		t.Errorf("expected ultra prompt appended to developer message")
	}
}

func TestInjectOpenAIChat_AppendsToArrayContentAsInputText(t *testing.T) {
	body := RawBody{
		"messages": []any{
			map[string]any{
				"role": "system",
				"content": []any{
					map[string]any{"type": "input_text", "text": "existing"},
				},
			},
		},
	}
	InjectOpenAIChat(body, LevelFull)
	content := body["messages"].([]any)[0].(map[string]any)["content"].([]any)
	if len(content) != 2 {
		t.Fatalf("expected appended part, got %d parts", len(content))
	}
	last := content[1].(map[string]any)
	if last["type"] != "input_text" || last["text"] != Prompts[LevelFull] {
		t.Errorf("expected new input_text part with full prompt, got %+v", last)
	}
}

func TestInjectOpenAIResponses_PrefersInstructionsString(t *testing.T) {
	body := RawBody{
		"instructions": "base instructions",
		"input": []any{
			map[string]any{"role": "user", "content": "hi"},
		},
	}
	InjectOpenAIResponses(body, LevelFull)
	got := body["instructions"].(string)
	if !strings.HasPrefix(got, "base instructions"+Sep) {
		t.Errorf("expected instructions extended, got: %q", got)
	}
	if !strings.HasSuffix(got, Prompts[LevelFull]) {
		t.Errorf("expected full prompt suffix")
	}
	// input[] should be untouched when instructions handled it
	first := body["input"].([]any)[0].(map[string]any)
	if first["role"] != "user" {
		t.Errorf("expected input[] untouched")
	}
}

func TestInjectOpenAIResponses_FallsBackToInputWhenNoInstructions(t *testing.T) {
	body := RawBody{
		"input": []any{
			map[string]any{"role": "user", "content": "hi"},
		},
	}
	InjectOpenAIResponses(body, LevelFull)
	input := body["input"].([]any)
	if len(input) != 2 {
		t.Fatalf("expected unshift, got %d items", len(input))
	}
	if input[0].(map[string]any)["role"] != "system" {
		t.Errorf("expected unshifted system message")
	}
}

func TestInjectAnthropic_StringSystemConcat(t *testing.T) {
	body := RawBody{"system": "base"}
	InjectAnthropic(body, LevelFull)
	got := body["system"].(string)
	if !strings.HasPrefix(got, "base"+Sep) || !strings.HasSuffix(got, Prompts[LevelFull]) {
		t.Errorf("expected base + SEP + full, got: %q", got)
	}
}

func TestInjectAnthropic_EmptyStringReplaced(t *testing.T) {
	body := RawBody{"system": ""}
	InjectAnthropic(body, LevelLite)
	if got := body["system"].(string); got != Prompts[LevelLite] {
		t.Errorf("expected lite prompt to replace empty system, got: %q", got)
	}
}

func TestInjectAnthropic_ArraySystemAppendsWhenNoCacheControl(t *testing.T) {
	body := RawBody{
		"system": []any{
			map[string]any{"type": "text", "text": "block 1"},
			map[string]any{"type": "text", "text": "block 2"},
		},
	}
	InjectAnthropic(body, LevelUltra)
	arr := body["system"].([]any)
	if len(arr) != 3 {
		t.Fatalf("expected append, got %d blocks", len(arr))
	}
	last := arr[2].(map[string]any)
	if last["type"] != "text" || last["text"] != Prompts[LevelUltra] {
		t.Errorf("expected ultra text block at tail, got: %+v", last)
	}
}

func TestInjectAnthropic_ArraySystemInsertsBeforeLastCacheControl(t *testing.T) {
	// Plan invariant: caveman must be inserted BEFORE the last cache_control
	// block so it stays inside the cached prefix.
	body := RawBody{
		"system": []any{
			map[string]any{"type": "text", "text": "block 1"},
			map[string]any{"type": "text", "text": "cached prefix", "cache_control": map[string]any{"type": "ephemeral"}},
		},
	}
	InjectAnthropic(body, LevelFull)
	arr := body["system"].([]any)
	if len(arr) != 3 {
		t.Fatalf("expected 3 blocks, got %d", len(arr))
	}
	// Order: [block 1, caveman, cached-prefix with cache_control]
	if text := arr[0].(map[string]any)["text"]; text != "block 1" {
		t.Errorf("arr[0] mismatch: %v", text)
	}
	mid := arr[1].(map[string]any)
	if mid["text"] != Prompts[LevelFull] {
		t.Errorf("expected caveman in middle, got: %+v", mid)
	}
	last := arr[2].(map[string]any)
	if _, has := last["cache_control"]; !has {
		t.Errorf("expected last block to retain cache_control: %+v", last)
	}
}

func TestInjectGemini_NewSystemInstructionDefault(t *testing.T) {
	body := RawBody{}
	InjectGemini(body, LevelFull)
	sys, ok := body["systemInstruction"].(map[string]any)
	if !ok {
		t.Fatalf("expected systemInstruction created")
	}
	parts := sys["parts"].([]any)
	if len(parts) != 1 {
		t.Fatalf("expected one part")
	}
	if parts[0].(map[string]any)["text"] != Prompts[LevelFull] {
		t.Errorf("expected caveman text in parts[0]")
	}
}

func TestInjectGemini_AppendsToExistingPartsCamelCase(t *testing.T) {
	body := RawBody{
		"systemInstruction": map[string]any{
			"parts": []any{map[string]any{"text": "base"}},
		},
	}
	InjectGemini(body, LevelUltra)
	parts := body["systemInstruction"].(map[string]any)["parts"].([]any)
	if len(parts) != 2 {
		t.Fatalf("expected 2 parts, got %d", len(parts))
	}
	if parts[1].(map[string]any)["text"] != Prompts[LevelUltra] {
		t.Errorf("expected ultra prompt appended")
	}
}

func TestInjectGemini_AppendsToExistingPartsSnakeCase(t *testing.T) {
	body := RawBody{
		"system_instruction": map[string]any{
			"parts": []any{map[string]any{"text": "base"}},
		},
	}
	InjectGemini(body, LevelLite)
	sys, ok := body["system_instruction"].(map[string]any)
	if !ok {
		t.Fatalf("expected snake_case key preserved")
	}
	parts := sys["parts"].([]any)
	if len(parts) != 2 {
		t.Fatalf("expected 2 parts, got %d", len(parts))
	}
}

func TestInjectGemini_HonorsRequestWrap(t *testing.T) {
	body := RawBody{
		"request": map[string]any{
			"systemInstruction": map[string]any{
				"parts": []any{map[string]any{"text": "base"}},
			},
		},
	}
	InjectGemini(body, LevelFull)
	req := body["request"].(map[string]any)
	parts := req["systemInstruction"].(map[string]any)["parts"].([]any)
	if len(parts) != 2 {
		t.Fatalf("expected 2 parts under body.request.systemInstruction, got %d", len(parts))
	}
}

func TestInject_NilBodyOrUnknownLevelNoOp(t *testing.T) {
	if got := InjectOpenAIChat(nil, LevelFull); got.Enabled {
		t.Errorf("nil body should not enable")
	}
	body := RawBody{"messages": []any{}}
	if got := InjectOpenAIChat(body, ""); got.Enabled {
		t.Errorf("empty level should not enable")
	}
	if got := InjectOpenAIChat(body, "bogus"); got.Enabled {
		t.Errorf("unknown level should not enable")
	}
}
