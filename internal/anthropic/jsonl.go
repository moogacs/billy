package anthropic

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/geekmonkey/billy/internal/model"
)

type lineEnvelope struct {
	Type          string          `json:"type"`
	UUID          string          `json:"uuid"`
	Timestamp     string          `json:"timestamp"`
	SessionID     string          `json:"sessionId"`
	UserType      string          `json:"userType"`
	Message       json.RawMessage `json:"message"`
	ToolUseResult json.RawMessage `json:"toolUseResult"`
}

type userMessage struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

type assistantMessage struct {
	Model string `json:"model"`
	ID    string `json:"id"`
	Role  string `json:"role"`
	Usage *struct {
		InputTokens              int `json:"input_tokens"`
		OutputTokens             int `json:"output_tokens"`
		CacheReadInputTokens     int `json:"cache_read_input_tokens"`
		CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
	} `json:"usage"`
}

// ReadEvents parses a Claude Code session JSONL file into normalized events.
func ReadEvents(path string) ([]model.NormalizedEvent, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	// Large lines
	const max = 64 * 1024 * 1024
	buf := make([]byte, max)
	sc.Buffer(buf, max)

	var out []model.NormalizedEvent
	lineNo := 0
	seenMsgID := map[string]struct{}{}

	for sc.Scan() {
		lineNo++
		line := sc.Bytes()
		if len(strings.TrimSpace(string(line))) == 0 {
			continue
		}
		var env lineEnvelope
		if err := json.Unmarshal(line, &env); err != nil {
			continue
		}
		switch env.Type {
		case "summary", "system", "file-history-snapshot":
			continue
		case "user":
			if !isRealUserPrompt(env) {
				continue
			}
			text, err := extractUserText(env.Message)
			if err != nil || strings.TrimSpace(text) == "" {
				continue
			}
			out = append(out, model.EventUser{
				Prompt: model.UserPrompt{
					Vendor:    model.VendorAnthropic,
					Text:      text,
					Timestamp: env.Timestamp,
					SourceRef: fmt.Sprintf("L%d:%s", lineNo, env.UUID),
				},
			})
		case "assistant":
			var am assistantMessage
			if err := json.Unmarshal(env.Message, &am); err != nil {
				continue
			}
			if am.Usage == nil {
				continue
			}
			if am.ID != "" {
				if _, ok := seenMsgID[am.ID]; ok {
					continue
				}
				seenMsgID[am.ID] = struct{}{}
			}
			u := model.UsageBreakdown{
				InputTokens:              am.Usage.InputTokens,
				OutputTokens:             am.Usage.OutputTokens,
				CacheReadInputTokens:     am.Usage.CacheReadInputTokens,
				CacheCreationInputTokens: am.Usage.CacheCreationInputTokens,
			}
			if u.InputTokens+u.OutputTokens+u.CacheReadInputTokens+u.CacheCreationInputTokens == 0 {
				continue
			}
			out = append(out, model.EventAssistant{
				Completion: model.AssistantCompletion{
					Vendor:    model.VendorAnthropic,
					Model:     am.Model,
					Timestamp: env.Timestamp,
					DedupKey:  am.ID,
					Usage:     u,
					SourceRef: fmt.Sprintf("L%d:%s", lineNo, env.UUID),
				},
			})
		default:
			continue
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func isRealUserPrompt(env lineEnvelope) bool {
	if len(env.ToolUseResult) > 0 && string(env.ToolUseResult) != "null" {
		return false
	}
	if env.UserType != "" && env.UserType != "external" {
		return false
	}
	return true
}

func extractUserText(raw json.RawMessage) (string, error) {
	var um userMessage
	if err := json.Unmarshal(raw, &um); err != nil {
		return "", err
	}
	if um.Role != "" && um.Role != "user" {
		return "", fmt.Errorf("not user role")
	}
	if len(um.Content) == 0 {
		return "", nil
	}
	// String content
	var s string
	if err := json.Unmarshal(um.Content, &s); err == nil {
		return s, nil
	}
	// Array of blocks (text, image, etc.)
	var blocks []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(um.Content, &blocks); err != nil {
		return "", err
	}
	var b strings.Builder
	for _, bl := range blocks {
		if bl.Type == "text" && bl.Text != "" {
			if b.Len() > 0 {
				b.WriteString("\n")
			}
			b.WriteString(bl.Text)
		}
	}
	return b.String(), nil
}
