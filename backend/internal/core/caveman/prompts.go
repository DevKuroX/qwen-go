// Package caveman ports 9router's natural-language compression hint
// (open-sse/rtk/caveman.js). Each level appends a SHARED_BOUNDARIES-bounded
// "respond tersely" instruction into the system prompt of the outgoing request.
package caveman

const (
	LevelLite  = "lite"
	LevelFull  = "full"
	LevelUltra = "ultra"
)

// Sep matches 9router's SEP between existing system text and the caveman block.
const Sep = "\n\n"

const sharedBoundaries = "Code blocks, file paths, commands, errors, URLs: keep exact. Security warnings, irreversible action confirmations, multi-step ordered sequences: write normal. Resume terse style after."

// Prompts mirrors 9router's CAVEMAN_PROMPTS verbatim — joined with single
// spaces, matching the JS .join(" ") pattern.
var Prompts = map[string]string{
	LevelLite: "Respond tersely. Keep grammar and full sentences but drop filler, hedging and pleasantries (just/really/basically/sure/of course/I'd be happy to). " +
		"Pattern: state the thing, the action, the reason. Then next step. " +
		sharedBoundaries + " " +
		"Active every response until user asks for normal mode.",

	LevelFull: "Respond like terse caveman. All technical substance stay exact, only fluff die. " +
		"Drop: articles (a/an/the), filler (just/really/basically/actually/simply), pleasantries, hedging. Fragments OK. Short synonyms (big not extensive, fix not implement a solution for). " +
		"Pattern: [thing] [action] [reason]. [next step]. " +
		sharedBoundaries + " " +
		"Active every response until user asks for normal mode.",

	LevelUltra: "Respond ultra-terse. Maximum compression. Telegraphic. " +
		"Abbreviate (DB/auth/config/req/res/fn/impl), strip conjunctions, use arrows for causality (X → Y). One word when one word enough. " +
		"Pattern: [thing] → [result]. [fix]. " +
		sharedBoundaries + " " +
		"Active every response until user asks for normal mode.",
}

// Prompt returns the caveman text for a level, or "" if the level is unknown.
// Callers should treat "" as "no-op, skip injection".
func Prompt(level string) string {
	return Prompts[level]
}
