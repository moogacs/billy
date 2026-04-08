package codex

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/geekmonkey/billy/internal/model"
)

// ReadEvents parses Codex CLI session JSONL (and compatible turn.completed lines).
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
	seenReq := map[string]struct{}{}

	for sc.Scan() {
		lineNo++
		line := sc.Bytes()
		if len(strings.TrimSpace(string(line))) == 0 {
			continue
		}
		var probe struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(line, &probe); err != nil {
			continue
		}
		if probe.Type == "turn.completed" {
			evs, err := parseTurnCompleted(line, lineNo, seenReq)
			if err != nil {
				continue
			}
			out = append(out, evs...)
			continue
		}
		evs, err := parseCodexMessageLine(line, lineNo, seenReq)
		if err != nil {
			continue
		}
		out = append(out, evs...)
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

type codexLine struct {
	Timestamp string `json:"timestamp"`
	SessionID string `json:"sessionId"`
	RequestID string `json:"requestId"`
	Message   struct {
		Role    string `json:"role"`
		ID      string `json:"id"`
		Model   string `json:"model"`
		Content json.RawMessage `json:"content"`
		Usage   *usageObj       `json:"usage"`
	} `json:"message"`
}

type usageObj struct {
	InputTokens              int `json:"input_tokens"`
	OutputTokens             int `json:"output_tokens"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
	CachedInputTokens        int `json:"cached_input_tokens"`
}

func parseCodexMessageLine(line []byte, lineNo int, seenReq map[string]struct{}) ([]model.NormalizedEvent, error) {
	var row codexLine
	if err := json.Unmarshal(line, &row); err != nil {
		return nil, err
	}
	var out []model.NormalizedEvent

	// User prompt
	if row.Message.Role == "user" {
		text := extractTextContent(row.Message.Content)
		if strings.TrimSpace(text) != "" {
			out = append(out, model.EventUser{
				Prompt: model.UserPrompt{
					Vendor:    model.VendorOpenAI,
					Text:      text,
					Timestamp: row.Timestamp,
					SourceRef: fmt.Sprintf("L%d", lineNo),
				},
			})
		}
		return out, nil
	}

	if row.Message.Usage == nil {
		return nil, fmt.Errorf("no usage")
	}
	if row.Message.Role != "" && row.Message.Role != "assistant" {
		return nil, fmt.Errorf("skip role %s", row.Message.Role)
	}
	u := model.UsageBreakdown{
		InputTokens:              row.Message.Usage.InputTokens,
		OutputTokens:             row.Message.Usage.OutputTokens,
		CacheReadInputTokens:     row.Message.Usage.CacheReadInputTokens + row.Message.Usage.CachedInputTokens,
		CacheCreationInputTokens: row.Message.Usage.CacheCreationInputTokens,
	}
	if u.InputTokens+u.OutputTokens+u.CacheReadInputTokens+u.CacheCreationInputTokens == 0 {
		return nil, fmt.Errorf("zero usage")
	}
	key := row.RequestID
	if key == "" {
		key = row.Message.ID
	}
	if key != "" {
		if _, ok := seenReq[key]; ok {
			return nil, fmt.Errorf("dup")
		}
		seenReq[key] = struct{}{}
	}
	out = append(out, model.EventAssistant{
		Completion: model.AssistantCompletion{
			Vendor:    model.VendorOpenAI,
			Model:     row.Message.Model,
			Timestamp: row.Timestamp,
			DedupKey:  key,
			Usage:     u,
			SourceRef: fmt.Sprintf("L%d", lineNo),
		},
	})
	return out, nil
}

func parseTurnCompleted(line []byte, lineNo int, seenReq map[string]struct{}) ([]model.NormalizedEvent, error) {
	var row struct {
		Type      string    `json:"type"`
		Timestamp string    `json:"timestamp"`
		RequestID string    `json:"requestId"`
		Model     string    `json:"model"`
		Usage     *usageObj `json:"usage"`
	}
	if err := json.Unmarshal(line, &row); err != nil || row.Usage == nil {
		return nil, fmt.Errorf("bad turn.completed")
	}
	u := model.UsageBreakdown{
		InputTokens:              row.Usage.InputTokens,
		OutputTokens:             row.Usage.OutputTokens,
		CacheReadInputTokens:     row.Usage.CacheReadInputTokens + row.Usage.CachedInputTokens,
		CacheCreationInputTokens: row.Usage.CacheCreationInputTokens,
	}
	if u.InputTokens+u.OutputTokens+u.CacheReadInputTokens+u.CacheCreationInputTokens == 0 {
		return nil, fmt.Errorf("zero")
	}
	key := row.RequestID
	if key == "" {
		key = fmt.Sprintf("L%d", lineNo)
	}
	if _, ok := seenReq[key]; ok {
		return nil, fmt.Errorf("dup")
	}
	seenReq[key] = struct{}{}
	return []model.NormalizedEvent{
		model.EventAssistant{
			Completion: model.AssistantCompletion{
				Vendor:    model.VendorOpenAI,
				Model:     row.Model,
				Timestamp: row.Timestamp,
				DedupKey:  key,
				Usage:     u,
				SourceRef: fmt.Sprintf("L%d", lineNo),
			},
		},
	}, nil
}

func extractTextContent(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	var blocks []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &blocks); err != nil {
		return ""
	}
	var b strings.Builder
	for _, bl := range blocks {
		if bl.Text != "" {
			if b.Len() > 0 {
				b.WriteString("\n")
			}
			b.WriteString(bl.Text)
		}
	}
	return b.String()
}

// DefaultHome returns CODEX_HOME or ~/.codex.
func DefaultHome() string {
	if h := os.Getenv("CODEX_HOME"); h != "" {
		return h
	}
	home, _ := os.UserHomeDir()
	if home == "" {
		home = os.Getenv("HOME")
	}
	return filepath.Join(home, ".codex")
}
