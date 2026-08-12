package config

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// LoadFromEnvFile loads configuration using values from path, falling back to
// the process environment and then MemoDrive defaults for unspecified keys.
func LoadFromEnvFile(path string) (*Config, error) {
	values, err := readEnvFile(path)
	if err != nil {
		return nil, err
	}
	return loadWithLookup(func(key string) string {
		if value, ok := values[key]; ok {
			return value
		}
		return os.Getenv(key)
	})
}

func readEnvFile(path string) (map[string]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open env file: %w", err)
	}
	defer file.Close()

	values := make(map[string]string)
	scanner := bufio.NewScanner(file)
	for lineNumber := 1; scanner.Scan(); lineNumber++ {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimSpace(strings.TrimPrefix(line, "export "))
		key, value, ok := strings.Cut(line, "=")
		if !ok || strings.TrimSpace(key) == "" {
			return nil, fmt.Errorf("env file line %d: expected KEY=VALUE", lineNumber)
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if len(value) >= 2 {
			if (value[0] == '\'' && value[len(value)-1] == '\'') ||
				(value[0] == '"' && value[len(value)-1] == '"') {
				value = value[1 : len(value)-1]
			}
		}
		values[key] = value
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read env file: %w", err)
	}
	return values, nil
}
