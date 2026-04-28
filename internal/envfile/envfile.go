package envfile

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const maxEnvLineBytes = 1 << 20

var getWorkingDirectory = os.Getwd
var getExecutablePath = os.Executable

type ApplyOptions struct {
	Override bool
}

func Resolve(path string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("env file path is required")
	}

	resolved, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve env file path: %w", err)
	}

	info, err := os.Stat(resolved)
	if err != nil {
		return "", fmt.Errorf("stat env file: %w", err)
	}
	if info.IsDir() {
		return "", fmt.Errorf("env file is a directory: %s", resolved)
	}

	return resolved, nil
}

func Load(path string) (map[string]string, error) {
	resolved, err := Resolve(path)
	if err != nil {
		return nil, err
	}

	file, err := os.Open(resolved)
	if err != nil {
		return nil, fmt.Errorf("open env file: %w", err)
	}
	defer file.Close()

	values := map[string]string{}
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 4096), maxEnvLineBytes)
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "export ") {
			line = strings.TrimSpace(strings.TrimPrefix(line, "export "))
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			return nil, fmt.Errorf("invalid env assignment in %s at line %d", resolved, lineNumber)
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" {
			return nil, fmt.Errorf("invalid env assignment in %s at line %d: missing key", resolved, lineNumber)
		}
		value = stripMatchingQuotes(value)
		values[key] = value
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read env file: %w", err)
	}

	return values, nil
}

func stripMatchingQuotes(value string) string {
	if len(value) < 2 {
		return value
	}

	if value[0] == '"' && value[len(value)-1] == '"' {
		return value[1 : len(value)-1]
	}
	if value[0] == '\'' && value[len(value)-1] == '\'' {
		return value[1 : len(value)-1]
	}

	return value
}

func Apply(path string, options ApplyOptions) (string, error) {
	resolved, err := Resolve(path)
	if err != nil {
		return "", err
	}

	values, err := Load(resolved)
	if err != nil {
		return "", err
	}

	for key, value := range values {
		if !options.Override {
			if _, exists := os.LookupEnv(key); exists {
				continue
			}
		}
		if err := os.Setenv(key, value); err != nil {
			return "", fmt.Errorf("set env var %s: %w", key, err)
		}
	}

	return resolved, nil
}

func ApplyDefaultIfPresent(options ApplyOptions) (string, error) {
	wd, err := getWorkingDirectory()
	if err != nil {
		return "", fmt.Errorf("get working directory: %w", err)
	}

	if resolved, ok, err := applyDefaultCandidate(filepath.Join(wd, ".env"), options); err != nil || ok {
		return resolved, err
	}

	executablePath, err := getExecutablePath()
	if err != nil {
		return "", fmt.Errorf("get executable path: %w", err)
	}

	resolved, _, err := applyDefaultCandidate(filepath.Join(filepath.Dir(executablePath), ".env"), options)
	return resolved, err
}

func applyDefaultCandidate(path string, options ApplyOptions) (string, bool, error) {
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return "", false, nil
		}
		return "", false, fmt.Errorf("stat default env file: %w", err)
	}

	resolved, err := Apply(path, options)
	if err != nil {
		return "", false, err
	}

	return resolved, true, nil
}
