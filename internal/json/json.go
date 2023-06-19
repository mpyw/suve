package json

import "encoding/json"

func Pretty(data string) string {
	var unmarshalled any
	if err := json.Unmarshal([]byte(data), &unmarshalled); err != nil {
		return data
	}
	marshalled, err := json.MarshalIndent(unmarshalled, "", "  ")
	if err != nil {
		return data
	}
	return string(marshalled)
}
