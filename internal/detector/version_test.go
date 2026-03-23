package detector

import "testing"

func TestParseVersion(t *testing.T) {
	tests := []struct {
		name     string
		output   string
		tool     string
		expected string
	}{
		{"version key", `{"version":"1.2.3"}`, "ddev", "1.2.3"},
		{"Version key", `{"Version":"2.0.0"}`, "ddev", "2.0.0"},
		{"tool-specific key", `{"ddev version":"3.0.0"}`, "ddev", "3.0.0"},
		{"raw key", `{"raw":"v4.0.0"}`, "ddev", "v4.0.0"},
		{"invalid JSON", `not json`, "ddev", "unknown"},
		{"empty object", `{}`, "ddev", "unknown"},
		{"surrounding whitespace", "  {\"version\":\"1.0.0\"}  ", "ddev", "1.0.0"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseVersion(tt.output, tt.tool)
			if got != tt.expected {
				t.Errorf("parseVersion(%q, %q) = %q, want %q", tt.output, tt.tool, got, tt.expected)
			}
		})
	}
}

func TestParseDdevVersionPlain(t *testing.T) {
	tests := []struct {
		name     string
		output   string
		expected string
	}{
		{"DDEV version prefix", "DDEV version v1.22.0", "v1.22.0"},
		{"ddev version prefix", "ddev version 1.22.0", "1.22.0"},
		{"version colon format", "version: 1.21.0", "1.21.0"},
		{"trailing comma stripped", "DDEV version v1.22.0,", "v1.22.0"},
		{"multiline with version line", "build date: 2024\nDDEV version v1.23.0\ngo version", "v1.23.0"},
		{"empty output", "", "unknown"},
		{"no matching line", "some other output\nno version here", "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseDdevVersionPlain(tt.output)
			if got != tt.expected {
				t.Errorf("parseDdevVersionPlain(%q) = %q, want %q", tt.output, got, tt.expected)
			}
		})
	}
}

func TestParseWardenVersion(t *testing.T) {
	tests := []struct {
		name     string
		output   string
		expected string
	}{
		{"Warden with version number", "Warden 0.15.0", "0.15.0"},
		{"warden lowercase", "warden version 0.15.0", "0.15.0"},
		{"trailing comma stripped", "Warden 0.15.0,", "0.15.0"},
		{"no version number", "Warden", "unknown"},
		{"empty output", "", "unknown"},
		{"unrelated output", "some other tool 1.0.0", "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseWardenVersion(tt.output)
			if got != tt.expected {
				t.Errorf("parseWardenVersion(%q) = %q, want %q", tt.output, got, tt.expected)
			}
		})
	}
}
