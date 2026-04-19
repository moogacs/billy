package proxy

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type UsageRecord struct {
	Time          time.Time `json:"time"`
	Command       string    `json:"command"`
	RawBytes      int       `json:"raw_bytes"`
	CompactBytes  int       `json:"compact_bytes"`
	RawTokens     int       `json:"raw_tokens"`
	CompactTokens int       `json:"compact_tokens"`
}

type GainSummary struct {
	Records       int     `json:"records"`
	RawTokens     int     `json:"raw_tokens"`
	CompactTokens int     `json:"compact_tokens"`
	TokensSaved   int     `json:"tokens_saved"`
	SavedPercent  float64 `json:"saved_percent"`
}

func appendUsage(rec UsageRecord) error {
	p, err := usagePath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return err
	}
	f, err := os.OpenFile(p, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	b, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	if _, err := f.Write(append(b, '\n')); err != nil {
		return err
	}
	return nil
}

func LoadGainSummary(since time.Time) (GainSummary, error) {
	p, err := usagePath()
	if err != nil {
		return GainSummary{}, err
	}
	f, err := os.Open(p)
	if err != nil {
		if os.IsNotExist(err) {
			return GainSummary{}, nil
		}
		return GainSummary{}, err
	}
	defer f.Close()

	var out GainSummary
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var rec UsageRecord
		if err := json.Unmarshal(sc.Bytes(), &rec); err != nil {
			continue
		}
		if !since.IsZero() && rec.Time.Before(since) {
			continue
		}
		out.Records++
		out.RawTokens += rec.RawTokens
		out.CompactTokens += rec.CompactTokens
	}
	if err := sc.Err(); err != nil {
		return GainSummary{}, err
	}
	out.TokensSaved = out.RawTokens - out.CompactTokens
	if out.RawTokens > 0 {
		out.SavedPercent = float64(out.TokensSaved) * 100 / float64(out.RawTokens)
	}
	return out, nil
}

func usagePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home dir: %w", err)
	}
	return filepath.Join(home, ".billy", "proxy-usage.jsonl"), nil
}

func estimateTokens(s string) int {
	n := len([]rune(s))
	if n == 0 {
		return 0
	}
	est := n / 4
	if n%4 != 0 {
		est++
	}
	return est
}
