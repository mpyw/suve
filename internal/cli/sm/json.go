package sm

import (
	"encoding/json"
)

// formatJSON formats a JSON string with indentation.
func formatJSON(value string) string {
	var data any
	if err := json.Unmarshal([]byte(value), &data); err != nil {
		return value
	}
	formatted, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return value
	}
	return string(formatted)
}
