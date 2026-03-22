package sample

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

type Profile struct {
	Name     string `json:"name"`
	Email    string `json:"email,omitempty"`
	Age      int    `json:"age,omitempty"`
	IsActive bool   `json:"is_active"`
}

func MarshalProfile(p Profile) (string, error) {
	data, err := json.Marshal(p)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func UnmarshalProfile(data string) (Profile, error) {
	var p Profile
	if err := json.Unmarshal([]byte(data), &p); err != nil {
		return Profile{}, fmt.Errorf("invalid json: %w", err)
	}
	return p, nil
}

func FormatMap(m map[string]string) string {
	if len(m) == 0 {
		return "{}"
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var sb strings.Builder
	sb.WriteString("{")
	for i, k := range keys {
		if i > 0 {
			sb.WriteString(", ")
		}
		sb.WriteString(fmt.Sprintf("%s=%s", k, m[k]))
	}
	sb.WriteString("}")
	return sb.String()
}

func CountKeys(m map[string]int) int {
	return len(m)
}
