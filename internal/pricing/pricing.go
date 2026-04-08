package pricing

import (
	_ "embed"
	"fmt"
	"os"
	"strings"

	"github.com/geekmonkey/billy/internal/model"
	"gopkg.in/yaml.v3"
)

//go:embed default.yaml
var defaultYAML []byte

// Table holds per-model rates for each vendor.
type Table struct {
	Version         int         `yaml:"version"`
	DisclaimerText  string      `yaml:"disclaimer"`
	Anthropic       vendorBlock `yaml:"anthropic"`
	OpenAI          vendorBlock `yaml:"openai"`
	Cursor          cursorBlock `yaml:"cursor"`
}

type vendorBlock struct {
	Models       map[string]modelRates `yaml:"models"`
	ModelAliases map[string]string     `yaml:"model_aliases"`
}

type cursorBlock struct {
	ModelAliases map[string]string `yaml:"model_aliases"`
}

type modelRates struct {
	InputPerMtokUSD              float64 `yaml:"input_per_mtok_usd"`
	OutputPerMtokUSD             float64 `yaml:"output_per_mtok_usd"`
	CacheReadInputPerMtokUSD     float64 `yaml:"cache_read_input_per_mtok_usd"`
	CacheCreationInputPerMtokUSD float64 `yaml:"cache_creation_input_per_mtok_usd"`
}

// Load reads pricing from path, or embedded default if path is empty.
func Load(path string) (*Table, error) {
	var raw []byte
	var err error
	if path == "" {
		raw = defaultYAML
	} else {
		raw, err = os.ReadFile(path)
		if err != nil {
			return nil, err
		}
	}
	var t Table
	if err := yaml.Unmarshal(raw, &t); err != nil {
		return nil, fmt.Errorf("parse pricing yaml: %w", err)
	}
	if t.Anthropic.Models == nil {
		t.Anthropic.Models = map[string]modelRates{}
	}
	if t.Anthropic.ModelAliases == nil {
		t.Anthropic.ModelAliases = map[string]string{}
	}
	if t.OpenAI.Models == nil {
		t.OpenAI.Models = map[string]modelRates{}
	}
	if t.OpenAI.ModelAliases == nil {
		t.OpenAI.ModelAliases = map[string]string{}
	}
	if t.Cursor.ModelAliases == nil {
		t.Cursor.ModelAliases = map[string]string{}
	}
	return &t, nil
}

// CostUSD returns estimated USD for usage under vendor/model. ok is false if model is unknown.
func (t *Table) CostUSD(v model.Vendor, modelID string, u model.UsageBreakdown) (usd float64, ok bool) {
	rates, vok := t.lookupRates(v, modelID)
	if !vok {
		return 0, false
	}
	const m = 1e6
	usd += float64(u.InputTokens) / m * rates.InputPerMtokUSD
	usd += float64(u.OutputTokens) / m * rates.OutputPerMtokUSD
	usd += float64(u.CacheReadInputTokens) / m * rates.CacheReadInputPerMtokUSD
	usd += float64(u.CacheCreationInputTokens) / m * rates.CacheCreationInputPerMtokUSD
	return usd, true
}

// anthropicRatesFallback uses published snapshot rows when logs only contain short model ids (e.g. claude-sonnet-4-6).
func anthropicRatesFallback(t *Table, modelID string) (modelRates, bool) {
	if strings.Contains(modelID, "claude-sonnet-4") {
		if r, ok := t.Anthropic.Models["claude-sonnet-4-5-20250929"]; ok {
			return r, true
		}
		if r, ok := t.Anthropic.Models["claude-sonnet-4-20250514"]; ok {
			return r, true
		}
	}
	if strings.Contains(modelID, "claude-opus-4") {
		if r, ok := t.Anthropic.Models["claude-opus-4-20250514"]; ok {
			return r, true
		}
	}
	if strings.Contains(modelID, "claude-haiku-4") {
		if r, ok := t.Anthropic.Models["claude-haiku-4-20251001"]; ok {
			return r, true
		}
	}
	return modelRates{}, false
}

func (t *Table) lookupRates(v model.Vendor, modelID string) (modelRates, bool) {
	switch v {
	case model.VendorAnthropic:
		if r, ok := t.Anthropic.Models[modelID]; ok {
			return r, true
		}
		if alias, ok := t.Anthropic.ModelAliases[modelID]; ok {
			if r, ok := t.Anthropic.Models[alias]; ok {
				return r, true
			}
		}
		return anthropicRatesFallback(t, modelID)
	case model.VendorOpenAI:
		if r, ok := t.OpenAI.Models[modelID]; ok {
			return r, true
		}
		if alias, ok := t.OpenAI.ModelAliases[modelID]; ok {
			if r, ok := t.OpenAI.Models[alias]; ok {
				return r, true
			}
		}
		return modelRates{}, false
	case model.VendorCursor:
		if alias, aok := t.Cursor.ModelAliases[modelID]; aok {
			if r, ok := t.OpenAI.Models[alias]; ok {
				return r, true
			}
		}
		if r, ok := t.OpenAI.Models[modelID]; ok {
			return r, true
		}
		return modelRates{}, false
	default:
		return modelRates{}, false
	}
}

// DisclaimerLine returns the YAML disclaimer or a default string.
func (t *Table) DisclaimerLine() string {
	if t.DisclaimerText != "" {
		return t.DisclaimerText
	}
	return "Token-based estimate; not your subscription or invoice."
}
