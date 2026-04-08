package cursor

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/geekmonkey/billy/internal/model"
)

// ReadEvents parses Cursor agent-transcript style JSONL (best effort).
// Token fields vary by Cursor version; this looks for common shapes.
func ReadEvents(path string) ([]model.NormalizedEvent, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	const max = 64 * 1024 * 1024
	buf := make([]byte, max)
	sc.Buffer(buf, max)

	var out []model.NormalizedEvent
	lineNo := 0
	seen := map[string]struct{}{}

	for sc.Scan() {
		lineNo++
		line := sc.Bytes()
		if len(strings.TrimSpace(string(line))) == 0 {
			continue
		}
		var raw map[string]json.RawMessage
		if err := json.Unmarshal(line, &raw); err != nil {
			continue
		}
		role := jsonString(raw["role"])
		if role == "" {
			role = jsonString(raw["type"])
		}

		switch role {
		case "user":
			text := extractContent(raw)
			if strings.TrimSpace(text) == "" {
				continue
			}
			out = append(out, model.EventUser{
				Prompt: model.UserPrompt{
					Vendor:    model.VendorCursor,
					Text:      text,
					Timestamp: jsonString(raw["timestamp"]),
					SourceRef: fmt.Sprintf("L%d", lineNo),
				},
			})
		case "assistant":
			u, ok := usageFromRaw(raw)
			if !ok {
				continue
			}
			key := jsonString(raw["id"])
			if key == "" {
				key = jsonString(raw["requestId"])
			}
			if key == "" {
				key = fmt.Sprintf("L%d", lineNo)
			}
			if _, dup := seen[key]; dup {
				continue
			}
			seen[key] = struct{}{}
			modelName := jsonString(raw["model"])
			if modelName == "" {
				modelName = jsonString(nestedRaw(raw, "message", "model"))
			}
			out = append(out, model.EventAssistant{
				Completion: model.AssistantCompletion{
					Vendor:    model.VendorCursor,
					Model:     modelName,
					Timestamp: jsonString(raw["timestamp"]),
					DedupKey:  key,
					Usage:     u,
					SourceRef: fmt.Sprintf("L%d", lineNo),
				},
			})
		default:
			// Some transcripts nest role under "message"
			if msg, ok := raw["message"]; ok {
				var inner map[string]json.RawMessage
				if json.Unmarshal(msg, &inner) == nil {
					r2 := jsonString(inner["role"])
					if r2 == "user" {
						text := extractContent(inner)
						if strings.TrimSpace(text) != "" {
							out = append(out, model.EventUser{
								Prompt: model.UserPrompt{
									Vendor:    model.VendorCursor,
									Text:      text,
									Timestamp: jsonString(raw["timestamp"]),
									SourceRef: fmt.Sprintf("L%d", lineNo),
								},
							})
						}
					}
					if r2 == "assistant" {
						u, ok := usageFromMessageMap(inner)
						if ok {
							key := jsonString(inner["id"])
							if key == "" {
								key = fmt.Sprintf("L%d", lineNo)
							}
							if _, dup := seen[key]; !dup {
								seen[key] = struct{}{}
								out = append(out, model.EventAssistant{
									Completion: model.AssistantCompletion{
										Vendor:    model.VendorCursor,
										Model:     jsonString(inner["model"]),
										Timestamp: jsonString(raw["timestamp"]),
										DedupKey:  key,
										Usage:     u,
										SourceRef: fmt.Sprintf("L%d", lineNo),
									},
								})
							}
						}
					}
				}
			}
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func jsonString(r json.RawMessage) string {
	if len(r) == 0 || string(r) == "null" {
		return ""
	}
	var s string
	_ = json.Unmarshal(r, &s)
	return s
}

func extractContent(raw map[string]json.RawMessage) string {
	if c, ok := raw["content"]; ok {
		var s string
		if json.Unmarshal(c, &s) == nil {
			return s
		}
		var parts []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		}
		if json.Unmarshal(c, &parts) == nil {
			var b strings.Builder
			for _, p := range parts {
				if p.Text != "" {
					if b.Len() > 0 {
						b.WriteString("\n")
					}
					b.WriteString(p.Text)
				}
			}
			return b.String()
		}
	}
	return ""
}

func usageFromRaw(raw map[string]json.RawMessage) (model.UsageBreakdown, bool) {
	if u, ok := raw["usage"]; ok {
		if ub, ok2 := parseUsageJSON(u); ok2 {
			return ub, true
		}
	}
	if m, ok := raw["message"]; ok {
		var inner map[string]json.RawMessage
		if json.Unmarshal(m, &inner) == nil {
			return usageFromMessageMap(inner)
		}
	}
	return model.UsageBreakdown{}, false
}

func usageFromMessageMap(inner map[string]json.RawMessage) (model.UsageBreakdown, bool) {
	if u, ok := inner["usage"]; ok {
		return parseUsageJSON(u)
	}
	return model.UsageBreakdown{}, false
}

func parseUsageJSON(u json.RawMessage) (model.UsageBreakdown, bool) {
	var o struct {
		InputTokens              int `json:"input_tokens"`
		OutputTokens             int `json:"output_tokens"`
		CacheReadInputTokens     int `json:"cache_read_input_tokens"`
		CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
		PromptTokens             int `json:"prompt_tokens"`
		CompletionTokens         int `json:"completion_tokens"`
		CachedInputTokens        int `json:"cached_input_tokens"`
		TotalTokens              int `json:"total_tokens"`
	}
	if err := json.Unmarshal(u, &o); err != nil {
		return model.UsageBreakdown{}, false
	}
	in := o.InputTokens
	if in == 0 {
		in = o.PromptTokens
	}
	out := o.OutputTokens
	if out == 0 {
		out = o.CompletionTokens
	}
	cr := o.CacheReadInputTokens + o.CachedInputTokens
	cc := o.CacheCreationInputTokens
	if in+out+cr+cc == 0 && o.TotalTokens > 0 {
		// No breakdown: treat all as input for a minimal estimate (weak)
		in = o.TotalTokens
	}
	if in+out+cr+cc == 0 {
		return model.UsageBreakdown{}, false
	}
	return model.UsageBreakdown{
		InputTokens:              in,
		OutputTokens:             out,
		CacheReadInputTokens:     cr,
		CacheCreationInputTokens: cc,
	}, true
}

func nestedRaw(raw map[string]json.RawMessage, keys ...string) json.RawMessage {
	cur := raw
	for i, k := range keys {
		v, ok := cur[k]
		if !ok {
			return nil
		}
		if i == len(keys)-1 {
			return v
		}
		var next map[string]json.RawMessage
		if json.Unmarshal(v, &next) != nil {
			return nil
		}
		cur = next
	}
	return nil
}
