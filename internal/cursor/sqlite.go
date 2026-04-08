package cursor

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/geekmonkey/billy/internal/model"
	_ "modernc.org/sqlite"
)

// ReadEventsFromSQLite scans Cursor globalStorage state.vscdb or chat store.db for JSON blobs
// that contain usage objects (experimental).
func ReadEventsFromSQLite(dbPath string) ([]model.NormalizedEvent, error) {
	if _, err := os.Stat(dbPath); err != nil {
		return nil, err
	}
	dsn := "file:" + filepath.ToSlash(dbPath) + "?mode=ro"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	rows, err := db.Query(`SELECT key, value FROM ItemTable`)
	if err != nil {
		return nil, fmt.Errorf("cursor sqlite: %w", err)
	}
	defer rows.Close()

	var events []model.NormalizedEvent
	seen := map[string]struct{}{}
	for rows.Next() {
		var key string
		var val []byte
		if err := rows.Scan(&key, &val); err != nil {
			continue
		}
		if !strings.Contains(key, "composer") && !strings.Contains(key, "bubble") && !strings.Contains(key, "chat") {
			continue
		}
		evs := scanValueForUsage(key, val, seen)
		events = append(events, evs...)
	}
	return events, rows.Err()
}

func scanValueForUsage(key string, val []byte, seen map[string]struct{}) []model.NormalizedEvent {
	s := strings.TrimSpace(string(val))
	if s == "" || s[0] != '{' && s[0] != '[' {
		return nil
	}
	var probe map[string]json.RawMessage
	if json.Unmarshal(val, &probe) != nil {
		return nil
	}
	var out []model.NormalizedEvent
	// Direct usage on object
	if u, ok := probe["usage"]; ok {
		if ub, ok2 := parseUsageFromRaw(u); ok2 {
			id := key
			if _, dup := seen[id]; dup {
				return nil
			}
			seen[id] = struct{}{}
			out = append(out, model.EventAssistant{
				Completion: model.AssistantCompletion{
					Vendor:    model.VendorCursor,
					Model:     jsonString(probe["model"]),
					DedupKey:  id,
					Usage:     ub,
					SourceRef: "sqlite:" + key,
				},
			})
		}
	}
	return out
}

func parseUsageFromRaw(u json.RawMessage) (model.UsageBreakdown, bool) {
	var o struct {
		InputTokens              int `json:"input_tokens"`
		OutputTokens             int `json:"output_tokens"`
		CacheReadInputTokens     int `json:"cache_read_input_tokens"`
		CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
		PromptTokens             int `json:"prompt_tokens"`
		CompletionTokens         int `json:"completion_tokens"`
	}
	if json.Unmarshal(u, &o) != nil {
		return model.UsageBreakdown{}, false
	}
	in, out := o.InputTokens, o.OutputTokens
	if in == 0 {
		in = o.PromptTokens
	}
	if out == 0 {
		out = o.CompletionTokens
	}
	if in+out+o.CacheReadInputTokens+o.CacheCreationInputTokens == 0 {
		return model.UsageBreakdown{}, false
	}
	return model.UsageBreakdown{
		InputTokens:              in,
		OutputTokens:             out,
		CacheReadInputTokens:     o.CacheReadInputTokens,
		CacheCreationInputTokens: o.CacheCreationInputTokens,
	}, true
}
