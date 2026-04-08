package turnbuilder

import (
	"strings"

	"github.com/geekmonkey/billy/internal/model"
)

// BuildTurns groups normalized events into turns: each EventUser starts a turn;
// following EventAssistant entries belong to that turn until the next EventUser.
func BuildTurns(events []model.NormalizedEvent) []model.Turn {
	var turns []model.Turn
	var cur *model.Turn

	flush := func() {
		if cur == nil {
			return
		}
		cur.Index = len(turns)
		turns = append(turns, *cur)
		cur = nil
	}

	for _, ev := range events {
		switch e := ev.(type) {
		case model.EventUser:
			flush()
			cur = &model.Turn{
				Prompt:      e.Prompt,
				Completions: nil,
			}
		case model.EventAssistant:
			if cur == nil {
				cur = &model.Turn{
					Prompt: model.UserPrompt{
						Vendor: e.Completion.Vendor,
						Text:   "(no preceding user prompt in log)",
					},
					Completions: nil,
				}
			}
			cur.Completions = append(cur.Completions, e.Completion)
		}
	}
	flush()
	return turns
}

// PromptPreview returns a short single-line preview of t.
func PromptPreview(t string, max int) string {
	t = strings.ReplaceAll(strings.TrimSpace(t), "\n", " ")
	if max <= 0 || len(t) <= max {
		return t
	}
	return t[:max] + "…"
}
