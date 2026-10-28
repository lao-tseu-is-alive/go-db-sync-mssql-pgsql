package config

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
)

var environmentNamePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// LoadDotEnv loads an optional dotenv file without overriding exported values.
//
// Blank lines and lines beginning with # are ignored. Assignments may begin
// with "export" and may use unquoted, single-quoted or Go-style double-quoted
// values. Values are not expanded, executed or logged.
func LoadDotEnv(path string) (bool, error) {
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("open dotenv file: %w", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 4096), 1024*1024)
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		if err := loadDotEnvLine(scanner.Text(), lineNumber); err != nil {
			return false, err
		}
	}
	if err := scanner.Err(); err != nil {
		return false, fmt.Errorf("read dotenv file: %w", err)
	}
	return true, nil
}

func loadDotEnvLine(line string, lineNumber int) error {
	line = strings.TrimSpace(line)
	if line == "" || strings.HasPrefix(line, "#") {
		return nil
	}
	if strings.HasPrefix(line, "export ") || strings.HasPrefix(line, "export\t") {
		line = strings.TrimSpace(line[len("export"):])
	}
	separator := strings.IndexByte(line, '=')
	if separator < 1 {
		return fmt.Errorf("parse dotenv line %d: expected NAME=VALUE", lineNumber)
	}
	name := strings.TrimSpace(line[:separator])
	if !environmentNamePattern.MatchString(name) {
		return fmt.Errorf("parse dotenv line %d: invalid variable name", lineNumber)
	}
	value, err := parseDotEnvValue(strings.TrimSpace(line[separator+1:]))
	if err != nil {
		return fmt.Errorf("parse dotenv line %d: %w", lineNumber, err)
	}
	if _, exists := os.LookupEnv(name); exists {
		return nil
	}
	if err := os.Setenv(name, value); err != nil {
		return fmt.Errorf("set dotenv variable on line %d: %w", lineNumber, err)
	}
	return nil
}

func parseDotEnvValue(value string) (string, error) {
	if value == "" {
		return "", nil
	}
	if value[0] == '\'' {
		if len(value) < 2 || value[len(value)-1] != '\'' {
			return "", errors.New("unterminated single-quoted value")
		}
		return value[1 : len(value)-1], nil
	}
	if value[0] == '"' {
		if len(value) < 2 || value[len(value)-1] != '"' {
			return "", errors.New("unterminated double-quoted value")
		}
		unquoted, err := strconv.Unquote(value)
		if err != nil {
			return "", fmt.Errorf("invalid double-quoted value: %w", err)
		}
		return unquoted, nil
	}
	return value, nil
}
