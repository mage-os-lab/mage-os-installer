package detector

import (
	"encoding/json"
	"strings"
)

// parseVersion attempts to extract a version string from JSON output.
func parseVersion(output string, tool string) string {
	var data map[string]interface{}
	if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &data); err != nil {
		return "unknown"
	}

	// Try common JSON keys
	for _, key := range []string{"version", "Version", tool + " version", "raw"} {
		if v, ok := data[key]; ok {
			if s, ok := v.(string); ok && s != "" {
				return s
			}
		}
	}
	return "unknown"
}
