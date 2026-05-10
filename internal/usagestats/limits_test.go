package usagestats_test

import (
	"os"
	"testing"

	"claude-mesh/internal/usagestats"
)

// TestResolveFromEnvDefaultMax20 verifies that with no env vars, the default plan is Max20.
func TestResolveFromEnvDefaultMax20(t *testing.T) {
	os.Unsetenv("CLAUDE_MESH_PLAN_TIER")
	os.Unsetenv("CLAUDE_MESH_5H_LIMIT_TOKENS")
	os.Unsetenv("CLAUDE_MESH_WEEK_LIMIT_TOKENS")

	p := usagestats.ResolveFromEnv()

	if p.Tier != "max20" {
		t.Errorf("Tier: want 'max20', got %q", p.Tier)
	}
	if p.Limit5h != 220_000 {
		t.Errorf("Limit5h: want 220_000, got %d", p.Limit5h)
	}
	// Weekly = 220_000 × 7 × 24 / 5 = 7_392_000
	if p.LimitWeek != 7_392_000 {
		t.Errorf("LimitWeek: want 7_392_000, got %d", p.LimitWeek)
	}
}

// TestResolveFromEnvProTier verifies that CLAUDE_MESH_PLAN_TIER=pro selects Pro plan.
func TestResolveFromEnvProTier(t *testing.T) {
	os.Setenv("CLAUDE_MESH_PLAN_TIER", "pro")
	defer os.Unsetenv("CLAUDE_MESH_PLAN_TIER")
	os.Unsetenv("CLAUDE_MESH_5H_LIMIT_TOKENS")
	os.Unsetenv("CLAUDE_MESH_WEEK_LIMIT_TOKENS")

	p := usagestats.ResolveFromEnv()

	if p.Tier != "pro" {
		t.Errorf("Tier: want 'pro', got %q", p.Tier)
	}
	if p.Limit5h != 19_000 {
		t.Errorf("Limit5h: want 19_000, got %d", p.Limit5h)
	}
	// Weekly = 19_000 × 7 × 24 / 5 = 638_400
	if p.LimitWeek != 638_400 {
		t.Errorf("LimitWeek: want 638_400, got %d", p.LimitWeek)
	}
}

// TestResolveFromEnvMax5Tier verifies Max5 plan limits.
func TestResolveFromEnvMax5Tier(t *testing.T) {
	os.Setenv("CLAUDE_MESH_PLAN_TIER", "max5")
	defer os.Unsetenv("CLAUDE_MESH_PLAN_TIER")
	os.Unsetenv("CLAUDE_MESH_5H_LIMIT_TOKENS")
	os.Unsetenv("CLAUDE_MESH_WEEK_LIMIT_TOKENS")

	p := usagestats.ResolveFromEnv()

	if p.Tier != "max5" {
		t.Errorf("Tier: want 'max5', got %q", p.Tier)
	}
	if p.Limit5h != 88_000 {
		t.Errorf("Limit5h: want 88_000, got %d", p.Limit5h)
	}
	// Weekly = 88_000 × 7 × 24 / 5 = 2_956_800
	if p.LimitWeek != 2_956_800 {
		t.Errorf("LimitWeek: want 2_956_800, got %d", p.LimitWeek)
	}
}

// TestResolveFromEnvOverride verifies that explicit token overrides take precedence.
func TestResolveFromEnvOverride(t *testing.T) {
	os.Unsetenv("CLAUDE_MESH_PLAN_TIER")
	os.Setenv("CLAUDE_MESH_5H_LIMIT_TOKENS", "50000")
	os.Setenv("CLAUDE_MESH_WEEK_LIMIT_TOKENS", "1000000")
	defer os.Unsetenv("CLAUDE_MESH_5H_LIMIT_TOKENS")
	defer os.Unsetenv("CLAUDE_MESH_WEEK_LIMIT_TOKENS")

	p := usagestats.ResolveFromEnv()

	if p.Limit5h != 50_000 {
		t.Errorf("Limit5h override: want 50_000, got %d", p.Limit5h)
	}
	if p.LimitWeek != 1_000_000 {
		t.Errorf("LimitWeek override: want 1_000_000, got %d", p.LimitWeek)
	}
}

// TestResolveFromEnvUnknownTierFallsBackToMax20 verifies unknown tier → max20.
func TestResolveFromEnvUnknownTierFallsBackToMax20(t *testing.T) {
	os.Setenv("CLAUDE_MESH_PLAN_TIER", "enterprise")
	defer os.Unsetenv("CLAUDE_MESH_PLAN_TIER")

	p := usagestats.ResolveFromEnv()

	if p.Tier != "max20" {
		t.Errorf("Tier: want 'max20' fallback for unknown, got %q", p.Tier)
	}
	if p.Limit5h != 220_000 {
		t.Errorf("Limit5h: want 220_000 for unknown tier fallback, got %d", p.Limit5h)
	}
}
