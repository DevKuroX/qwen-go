package rtk

import (
	"testing"
)

func TestCompressOpenAIChat_StringToolMessage(t *testing.T) {
	big := makeLongDiff()
	body := RawBody{
		"messages": []any{
			map[string]any{"role": "tool", "tool_call_id": "call_1", "content": big},
		},
	}
	stats := CompressOpenAIChat(body)
	if len(stats.Hits) == 0 {
		t.Fatalf("expected at least one hit")
	}
	msg := body["messages"].([]any)[0].(map[string]any)
	got := msg["content"].(string)
	if len(got) >= len(big) {
		t.Errorf("expected in-place compression: got=%d big=%d", len(got), len(big))
	}
	if stats.BytesBefore <= stats.BytesAfter {
		t.Errorf("expected bytesBefore>bytesAfter, got %d/%d", stats.BytesBefore, stats.BytesAfter)
	}
}

func TestCompressOpenAIChat_ArrayToolMessage(t *testing.T) {
	big := makeLongDiff()
	body := RawBody{
		"messages": []any{
			map[string]any{
				"role": "tool",
				"content": []any{
					map[string]any{"type": "text", "text": big},
					map[string]any{"type": "text", "text": "short stays short"},
				},
			},
		},
	}
	stats := CompressOpenAIChat(body)
	if len(stats.Hits) == 0 {
		t.Fatalf("expected hit on array-form tool content")
	}
	parts := body["messages"].([]any)[0].(map[string]any)["content"].([]any)
	bigOut := parts[0].(map[string]any)["text"].(string)
	shortOut := parts[1].(map[string]any)["text"].(string)
	if len(bigOut) >= len(big) {
		t.Errorf("expected big part compressed")
	}
	if shortOut != "short stays short" {
		t.Errorf("expected short part unchanged, got: %q", shortOut)
	}
}

func TestCompressAnthropic_StringToolResult(t *testing.T) {
	big := makeLongDiff()
	body := RawBody{
		"messages": []any{
			map[string]any{
				"role": "user",
				"content": []any{
					map[string]any{
						"type":        "tool_result",
						"tool_use_id": "toolu_1",
						"content":     big,
					},
				},
			},
		},
	}
	stats := CompressAnthropic(body)
	if len(stats.Hits) == 0 {
		t.Fatalf("expected hit on Claude string tool_result")
	}
	block := body["messages"].([]any)[0].(map[string]any)["content"].([]any)[0].(map[string]any)
	got := block["content"].(string)
	if len(got) >= len(big) {
		t.Errorf("expected compression: got=%d big=%d", len(got), len(big))
	}
}

func TestCompressAnthropic_ArrayToolResultTextParts(t *testing.T) {
	big := makeLongDiff()
	body := RawBody{
		"messages": []any{
			map[string]any{
				"role": "user",
				"content": []any{
					map[string]any{
						"type":        "tool_result",
						"tool_use_id": "toolu_1",
						"content": []any{
							map[string]any{"type": "text", "text": big},
							map[string]any{"type": "text", "text": "unchanged short"},
						},
					},
				},
			},
		},
	}
	stats := CompressAnthropic(body)
	if len(stats.Hits) == 0 {
		t.Fatalf("expected hit on Claude array tool_result")
	}
	parts := body["messages"].([]any)[0].(map[string]any)["content"].([]any)[0].(map[string]any)["content"].([]any)
	if parts[1].(map[string]any)["text"].(string) != "unchanged short" {
		t.Errorf("expected short part unchanged")
	}
	if len(parts[0].(map[string]any)["text"].(string)) >= len(big) {
		t.Errorf("expected big part compressed")
	}
}

func TestCompressAnthropic_SkipsIsErrorToolResult(t *testing.T) {
	big := makeLongDiff()
	body := RawBody{
		"messages": []any{
			map[string]any{
				"role": "user",
				"content": []any{
					map[string]any{
						"type":        "tool_result",
						"tool_use_id": "toolu_1",
						"content":     big,
						"is_error":    true,
					},
				},
			},
		},
	}
	stats := CompressAnthropic(body)
	if len(stats.Hits) != 0 {
		t.Errorf("expected no hits on is_error tool_result, got %d", len(stats.Hits))
	}
	block := body["messages"].([]any)[0].(map[string]any)["content"].([]any)[0].(map[string]any)
	if block["content"].(string) != big {
		t.Errorf("expected is_error content untouched")
	}
}

func TestCompressOpenAIResponses_FunctionCallOutputString(t *testing.T) {
	big := makeLongDiff()
	body := RawBody{
		"input": []any{
			map[string]any{
				"type":   "function_call_output",
				"output": big,
			},
		},
	}
	stats := CompressOpenAIResponses(body)
	if len(stats.Hits) == 0 {
		t.Fatalf("expected hit on function_call_output string")
	}
	got := body["input"].([]any)[0].(map[string]any)["output"].(string)
	if len(got) >= len(big) {
		t.Errorf("expected compression")
	}
}

func TestCompressOpenAIResponses_FunctionCallOutputArray(t *testing.T) {
	big := makeLongDiff()
	body := RawBody{
		"input": []any{
			map[string]any{
				"type": "function_call_output",
				"output": []any{
					map[string]any{"type": "input_text", "text": big},
				},
			},
		},
	}
	stats := CompressOpenAIResponses(body)
	if len(stats.Hits) == 0 {
		t.Fatalf("expected hit on function_call_output array")
	}
	parts := body["input"].([]any)[0].(map[string]any)["output"].([]any)
	got := parts[0].(map[string]any)["text"].(string)
	if len(got) >= len(big) {
		t.Errorf("expected compression on input_text part")
	}
}

func TestCompressGemini_NoOp(t *testing.T) {
	big := makeLongDiff()
	body := RawBody{
		"contents": []any{
			map[string]any{
				"role": "user",
				"parts": []any{
					map[string]any{"text": big},
				},
			},
		},
	}
	stats := CompressGemini(body)
	if len(stats.Hits) != 0 {
		t.Errorf("expected no-op on Gemini, got %d hits", len(stats.Hits))
	}
}

func TestCompressOpenAIChat_NilBodyReturnsEmptyStats(t *testing.T) {
	stats := CompressOpenAIChat(nil)
	if stats == nil {
		t.Fatalf("expected non-nil stats")
	}
	if len(stats.Hits) != 0 || stats.BytesBefore != 0 {
		t.Errorf("expected empty stats")
	}
}

func TestCompressOpenAIChat_MixedShapesNoCrash(t *testing.T) {
	body := RawBody{
		"messages": []any{
			map[string]any{"role": "system", "content": "you are"},
			map[string]any{"role": "user", "content": "hi"},
			map[string]any{"role": "assistant", "content": nil},
			map[string]any{"role": "tool", "tool_call_id": "c1", "content": makeGrepOutput()},
			map[string]any{"role": "user", "content": []any{
				map[string]any{"type": "text", "text": "next"},
			}},
		},
	}
	stats := CompressOpenAIChat(body)
	if stats == nil {
		t.Fatalf("expected stats")
	}
	if len(stats.Hits) == 0 {
		t.Errorf("expected at least one hit on the tool message")
	}
}
