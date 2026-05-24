// Package geminiweb implements a client for the Google Gemini web app API.
// This is a reverse-engineered protocol used by gemini.google.com — NOT the
// official Gemini API. Ported wholesale from /home/ubuntu/ai_proxy/backend/
// internal/geminiweb/ (parser-bug fixes intact); adapted to consult the
// in-memory ProviderRegistry for model IDs so they can be re-tuned from the
// dashboard when Google rotates them.
package geminiweb

import "time"

type session struct {
	Secure1PSID   string
	Secure1PSIDTS string

	AccessToken string

	BuildLabel string
	SessionID  string
	Language   string

	Proxy string

	client      *httpClient
	reqCounter  int
	lastRefresh time.Time

	// modelLookup is injected by the Provider so the session can map model
	// names to IDs without a hard-coded table. Falls back to defaultModels.
	modelLookup func(string) modelMeta
}

type modelMeta struct {
	ModelName string
	ModelID   string
	Capacity  int
}

// Fallback table used only when the registry has no model entry. Matches
// the model IDs documented in MULTI_PROVIDER_DESIGN.md §6.5; will go stale
// when Google rotates them — that's what the registry override is for.
var defaultModels = map[string]modelMeta{
	"gemini-3-flash":          {ModelName: "gemini-3-flash", ModelID: "fbb127bbb056c959", Capacity: 1},
	"gemini-3-flash-thinking": {ModelName: "gemini-3-flash-thinking", ModelID: "5bf011840784117a", Capacity: 1},
	"gemini-3-pro":            {ModelName: "gemini-3-pro", ModelID: "9d8ca3786ebdfbea", Capacity: 1},
}

type geminiResponse struct {
	Text     string
	Metadata []string
	RCID     string
	Done     bool
	// Images — generated/edited image URLs harvested from candidate_data[12].
	// Populated for image-capable models (e.g. gemini-3-flash, nano-banana).
	Images []generatedImage
}

type generatedImage struct {
	URL string
	Alt string
}

const (
	endpointGoogle   = "https://www.google.com"
	endpointInit     = "https://gemini.google.com/app"
	endpointGenerate = "https://gemini.google.com/_/BardChatUi/data/assistant.lamda.BardFrontendService/StreamGenerate"
	endpointRotate   = "https://accounts.google.com/RotateCookies"
	endpointUpload   = "https://content-push.googleapis.com/upload"

	streamingFlagIndex = 7
	gemFlagIndex       = 19
)

var defaultMetadata = []interface{}{"", "", "", nil, nil, nil, nil, nil, nil, ""}
