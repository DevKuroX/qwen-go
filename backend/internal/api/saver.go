package api

import (
	"bytes"
	"encoding/json"

	"github.com/qwenpi/qwenpi-go/internal/core"
	"github.com/qwenpi/qwenpi-go/internal/core/caveman"
	"github.com/qwenpi/qwenpi-go/internal/core/rtk"
)

// Format identifiers used by applySaver — kept symmetric with 9router's
// FORMATS constants so the dispatch reads the same way.
const (
	formatOpenAIChat      = "openai-chat"
	formatOpenAIResponses = "openai-responses"
	formatAnthropic       = "anthropic"
	formatGemini          = "gemini"
)

// applySaver runs RTK + Caveman on the raw request body, mutating in place
// according to format. Returns the (possibly) rewritten body plus accumulated
// stats for the request log. On any error the original body is returned
// untouched with empty stats — never failing closed.
func applySaver(rawBody []byte, format string) ([]byte, core.RequestSaverStats) {
	stats := core.RequestSaverStats{}
	if len(rawBody) == 0 || core.GlobalSettingsManager == nil {
		return rawBody, stats
	}
	rtkOn, cavOn, level := core.GlobalSettingsManager.SaverFlags()
	if !rtkOn && !cavOn {
		return rawBody, stats
	}

	dec := json.NewDecoder(bytes.NewReader(rawBody))
	dec.UseNumber()
	var body map[string]any
	if err := dec.Decode(&body); err != nil {
		return rawBody, stats
	}

	if rtkOn {
		switch format {
		case formatOpenAIChat:
			stats.RTK = rtk.CompressOpenAIChat(body)
		case formatOpenAIResponses:
			stats.RTK = rtk.CompressOpenAIResponses(body)
		case formatAnthropic:
			stats.RTK = rtk.CompressAnthropic(body)
		case formatGemini:
			stats.RTK = rtk.CompressGemini(body)
		}
	}

	if cavOn && level != "" {
		switch format {
		case formatOpenAIChat:
			stats.Caveman = caveman.InjectOpenAIChat(body, level)
		case formatOpenAIResponses:
			stats.Caveman = caveman.InjectOpenAIResponses(body, level)
		case formatAnthropic:
			stats.Caveman = caveman.InjectAnthropic(body, level)
		case formatGemini:
			stats.Caveman = caveman.InjectGemini(body, level)
		}
	}

	out, err := json.Marshal(body)
	if err != nil {
		return rawBody, core.RequestSaverStats{}
	}
	return out, stats
}
