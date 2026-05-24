package kiro

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/qwenpi/qwenpi-go/internal/models"
)

const defaultProfileArn = "arn:aws:codewhisperer:us-east-1:638616132270:profile/AAAACCCCXXXX"

// buildPayload converts an OpenAI-style ChatRequest into the AWS
// CodeWhisperer generateAssistantResponse JSON shape. Ported from
// /home/ubuntu/ai_proxy/_ref/9router/open-sse/translator/request/openai-to-kiro.js.
//
// Simplifications vs reference: no image support (v1), no tool-use round-
// trip (Kiro tool_use → OpenAI tool_calls is in the response translator;
// the request side here only flattens text).
func buildPayload(req *models.ChatRequest, model string, acc *models.Account) map[string]interface{} {
	history, current := convertMessages(req.Messages, model)

	// Prepend timestamp context — matches Kiro IDE's own behavior so the
	// assistant has wall-clock awareness without a system prompt round-trip.
	finalContent := ""
	if current != nil {
		finalContent = current.content
	}
	finalContent = fmt.Sprintf("[Context: Current time is %s]\n\n%s",
		time.Now().UTC().Format(time.RFC3339), finalContent)

	profileArn := defaultProfileArn
	if acc != nil && acc.Metadata != nil {
		if arn := acc.Metadata["profile_arn"]; arn != "" {
			profileArn = arn
		}
	}

	userMsg := map[string]interface{}{
		"content": finalContent,
		"modelId": model,
		"origin":  "AI_EDITOR",
	}

	payload := map[string]interface{}{
		"conversationState": map[string]interface{}{
			"chatTriggerType": "MANUAL",
			"conversationId":  uuidv4(),
			"currentMessage": map[string]interface{}{
				"userInputMessage": userMsg,
			},
			"history": history,
		},
	}
	if profileArn != "" {
		payload["profileArn"] = profileArn
	}

	inference := map[string]interface{}{
		"maxTokens": 32000,
	}
	if req.Temperature > 0 {
		inference["temperature"] = req.Temperature
	}
	if req.MaxTokens > 0 {
		inference["maxTokens"] = req.MaxTokens
	}
	payload["inferenceConfig"] = inference

	return payload
}

// historyMsg is the intermediate representation while walking the OpenAI
// messages array. We track role + accumulated content per turn and emit a
// userInputMessage/assistantResponseMessage at each role boundary.
type historyMsg struct {
	role    string
	content string
}

// convertMessages flattens OpenAI's role-tagged message list into Kiro's
// alternating user/assistant history + a separate currentMessage (the last
// user turn). Rules ported from the JS reference:
//   - "system" and "tool" roles fold into "user"
//   - consecutive same-role messages merge with "\n\n"
//   - the final user turn is popped off history and becomes currentMessage
func convertMessages(messages []models.ChatMessage, model string) ([]map[string]interface{}, *historyMsg) {
	var combined []historyMsg
	for _, m := range messages {
		role := m.Role
		if role == "system" || role == "tool" {
			role = "user"
		}
		if role != "user" && role != "assistant" {
			role = "user"
		}
		content := m.Content
		if content == "" {
			continue
		}
		if len(combined) > 0 && combined[len(combined)-1].role == role {
			combined[len(combined)-1].content += "\n\n" + content
			continue
		}
		combined = append(combined, historyMsg{role: role, content: content})
	}

	// Pop the LAST user turn — that becomes currentMessage. If none exists,
	// synthesize a "continue" placeholder (matches reference behavior).
	var current *historyMsg
	for i := len(combined) - 1; i >= 0; i-- {
		if combined[i].role == "user" {
			c := combined[i]
			current = &c
			combined = append(combined[:i], combined[i+1:]...)
			break
		}
	}
	if current == nil {
		current = &historyMsg{role: "user", content: "continue"}
	}

	// Emit history as Kiro turn shapes. Defensive default for empty
	// assistant content matches reference ("..." placeholder).
	history := make([]map[string]interface{}, 0, len(combined))
	for _, m := range combined {
		switch m.role {
		case "user":
			content := strings.TrimSpace(m.content)
			if content == "" {
				content = "continue"
			}
			history = append(history, map[string]interface{}{
				"userInputMessage": map[string]interface{}{
					"content": content,
					"modelId": model,
				},
			})
		case "assistant":
			content := strings.TrimSpace(m.content)
			if content == "" {
				content = "..."
			}
			history = append(history, map[string]interface{}{
				"assistantResponseMessage": map[string]interface{}{
					"content": content,
				},
			})
		}
	}
	return history, current
}

// uuidv4 generates an RFC 4122 v4 UUID without pulling in google/uuid.
// Used for conversationId and Amz-Sdk-Invocation-Id headers.
func uuidv4() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant 10
	hex := hex.EncodeToString(b[:])
	return hex[0:8] + "-" + hex[8:12] + "-" + hex[12:16] + "-" + hex[16:20] + "-" + hex[20:32]
}
