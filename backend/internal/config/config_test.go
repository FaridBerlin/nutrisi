package config

import (
	"os"
	"testing"
	"time"
)

// t.Setenv sets a variable for this test only and restores it afterwards.

// unset removes a variable for one test. t.Setenv first remembers the old
// value, so it is restored when the test ends.
func unset(t *testing.T, key string) {
	t.Helper()
	t.Setenv(key, "")
	os.Unsetenv(key)
}

func TestDefaults(t *testing.T) {
	for _, k := range []string{"PORT", "LLM_MODEL", "LLM_REASONING_EFFORT", "LLM_TIMEOUT"} {
		unset(t, k) // make sure nothing from the real environment leaks in
	}

	c := Load()
	if c.Port != "8080" || c.LLMModel != "qwen3.5:0.8b" || c.LLMTimeout != 3*time.Minute {
		t.Errorf("unexpected defaults: %+v", c)
	}
	if c.LLMReasoningEffort != "none" {
		t.Errorf("missing LLM_REASONING_EFFORT should default to none, got %q", c.LLMReasoningEffort)
	}
}

func TestEmptyReasoningEffortMeansModelDefault(t *testing.T) {
	t.Setenv("LLM_REASONING_EFFORT", "")
	if got := Load().LLMReasoningEffort; got != "" {
		t.Errorf("got %q, want empty", got)
	}
}

func TestDurationParsing(t *testing.T) {
	t.Setenv("LLM_TIMEOUT", "90s")
	if got := Load().LLMTimeout; got != 90*time.Second {
		t.Errorf("got %s", got)
	}
	t.Setenv("LLM_TIMEOUT", "soon")
	if got := Load().LLMTimeout; got != 3*time.Minute {
		t.Errorf("invalid value should fall back, got %s", got)
	}
}
