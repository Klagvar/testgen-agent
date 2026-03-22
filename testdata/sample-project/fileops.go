// Tests: file I/O pattern, env var pattern, AST FilterExecutableLines
package sample

import (
	"os"
	"path/filepath"
	"strings"
)

// ReadConfig reads a config file and returns key=value pairs.
func ReadConfig(path string) (map[string]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	result := make(map[string]string)
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			result[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
		}
	}
	return result, nil
}

// WriteConfig writes key=value pairs to a file.
func WriteConfig(path string, config map[string]string) error {
	var sb strings.Builder
	for k, v := range config {
		sb.WriteString(k + "=" + v + "\n")
	}
	return os.WriteFile(path, []byte(sb.String()), 0644)
}

// ListGoFiles returns all .go files in a directory.
func ListGoFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var files []string
	for _, e := range entries {
		if !e.IsDir() && filepath.Ext(e.Name()) == ".go" {
			files = append(files, e.Name())
		}
	}
	return files, nil
}

// GetEnvOrDefault returns env var value or a default.
func GetEnvOrDefault(key, defaultVal string) string {
	val := os.Getenv(key)
	if val == "" {
		return defaultVal
	}
	return val
}

// LoadDatabaseURL reads DATABASE_URL from env.
func LoadDatabaseURL() string {
	url, ok := os.LookupEnv("DATABASE_URL")
	if !ok || url == "" {
		return "postgres://localhost:5432/dev"
	}
	return url
}
