package app

import "testing"

func TestConfigValidateRequiresServiceNamesAndPorts(t *testing.T) {
	cfg := Config{}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected validation error for empty config")
	}
}
