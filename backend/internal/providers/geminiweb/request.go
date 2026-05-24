package geminiweb

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
)

// BuildChatPayload builds the form-encoded request body + URL for one chat
// request. Returns (url, body, error). The shape of the inner 69-element
// JSON array and which indices carry which flags come from reverse-engineering
// gemini.google.com's web client.
func (s *session) BuildChatPayload(prompt, modelName string) (string, string, error) {
	return s.BuildChatPayloadWithFiles(prompt, modelName, nil)
}

// BuildChatPayloadWithFiles is the file-aware variant. `fileData` is the
// already-formatted [[ [url], filename ], …] list that gemini's web client
// places at message_content[3] for image edits / multi-modal prompts. Pass
// nil for plain text or text-to-image generation (no input attachments).
func (s *session) BuildChatPayloadWithFiles(prompt, modelName string, fileData []interface{}) (string, string, error) {
	modelCfg := s.resolveModel(modelName)

	s.reqCounter += 100000
	reqID := s.reqCounter

	// message_content layout (matches python HanaokaYuzu lib):
	//   [0] prompt text
	//   [1] reserved
	//   [2] reserved
	//   [3] file_data — only set for edits / attachments
	//   [4..5] reserved
	//   [6] flag
	messageContent := []interface{}{prompt, 0, nil, nil, nil, nil, 0}
	if len(fileData) > 0 {
		messageContent[3] = fileData
	}

	innerReq := make([]interface{}, 69)
	innerReq[0] = messageContent
	innerReq[1] = []interface{}{s.Language}
	innerReq[2] = defaultMetadata
	innerReq[6] = []interface{}{1}
	innerReq[streamingFlagIndex] = 1
	innerReq[10] = 1
	innerReq[11] = 0
	innerReq[17] = [][]int{{0}}
	innerReq[18] = 0
	innerReq[27] = 1
	innerReq[30] = []int{4}
	innerReq[41] = []int{1}
	innerReq[53] = 0
	innerReq[61] = []interface{}{}
	innerReq[68] = 2

	innerReq[59] = strings.ToUpper(newUUID())

	innerJSON, err := json.Marshal(innerReq)
	if err != nil {
		return "", "", fmt.Errorf("failed to marshal inner req: %w", err)
	}
	freqPayload := []interface{}{nil, string(innerJSON)}
	freqJSON, err := json.Marshal(freqPayload)
	if err != nil {
		return "", "", fmt.Errorf("failed to marshal f.req: %w", err)
	}

	formData := url.Values{}
	formData.Set("at", s.AccessToken)
	formData.Set("f.req", string(freqJSON))

	queryParams := url.Values{}
	queryParams.Set("hl", s.Language)
	queryParams.Set("_reqid", fmt.Sprintf("%d", reqID))
	queryParams.Set("rt", "c")
	if s.BuildLabel != "" {
		queryParams.Set("bl", s.BuildLabel)
	}
	if s.SessionID != "" {
		queryParams.Set("f.sid", s.SessionID)
	}

	requestURL := endpointGenerate + "?" + queryParams.Encode()
	body := formData.Encode()

	_ = modelCfg // headers built separately via buildRequestHeaders
	return requestURL, body, nil
}

// buildRequestHeaders returns the request headers expected by the Gemini web
// frontend. modelHeader embeds the upstream model UUID into `x-goog-ext-525001261-jspb`.
func (s *session) buildRequestHeaders(modelName, uuidStr string) map[string]string {
	cfg := s.resolveModel(modelName)
	modelHeader := fmt.Sprintf(`[1,null,null,null,"%s",null,null,0,[4],null,null,%d]`, cfg.ModelID, cfg.Capacity)

	return map[string]string{
		"Content-Type":              "application/x-www-form-urlencoded;charset=utf-8",
		"Origin":                    "https://gemini.google.com",
		"Referer":                   "https://gemini.google.com/",
		"x-goog-ext-525001261-jspb": modelHeader,
		"x-goog-ext-73010989-jspb":  "[0]",
		"x-goog-ext-73010990-jspb":  "[0]",
		"x-goog-ext-525005358-jspb": fmt.Sprintf(`["%s",1]`, uuidStr),
		"X-Same-Domain":             "1",
	}
}

// newUUID produces a UUID v4-style hex string. We avoid pulling in google/uuid
// just for this one helper.
func newUUID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%s-%s-%s-%s-%s",
		hex.EncodeToString(b[0:4]),
		hex.EncodeToString(b[4:6]),
		hex.EncodeToString(b[6:8]),
		hex.EncodeToString(b[8:10]),
		hex.EncodeToString(b[10:16]),
	)
}
