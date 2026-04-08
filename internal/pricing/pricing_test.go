package pricing

import (
	"testing"

	"github.com/geekmonkey/billy/internal/model"
)

func TestCostUSD_claudeSonnet46(t *testing.T) {
	pt, err := Load("")
	if err != nil {
		t.Fatal(err)
	}
	u := model.UsageBreakdown{
		InputTokens:              1_000_000,
		OutputTokens:             1_000_000,
		CacheReadInputTokens:     1_000_000,
		CacheCreationInputTokens: 1_000_000,
	}
	usd, ok := pt.CostUSD(model.VendorAnthropic, "claude-sonnet-4-6", u)
	if !ok {
		t.Fatal("expected known model")
	}
	// 3+15+0.30+3.75 per 1M each
	want := 3.0 + 15.0 + 0.30 + 3.75
	if usd < want-1e-6 || usd > want+1e-6 {
		t.Fatalf("got %v want %v", usd, want)
	}
}

func TestCostUSD_anthropicFallbackShortID(t *testing.T) {
	pt, err := Load("")
	if err != nil {
		t.Fatal(err)
	}
	u := model.UsageBreakdown{InputTokens: 1_000_000}
	usd, ok := pt.CostUSD(model.VendorAnthropic, "claude-sonnet-4-99-future", u)
	if !ok {
		t.Fatal("expected fallback")
	}
	if usd < 2.99 || usd > 3.01 {
		t.Fatalf("got %v want ~3", usd)
	}
}
