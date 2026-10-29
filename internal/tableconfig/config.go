// Package tableconfig loads the strict multi-table selection configuration.
package tableconfig

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/lao-tseu-is-alive/go-db-sync-mssql-pgsql/internal/ident"
	"gopkg.in/yaml.v3"
)

const maxConfigBytes = 1 << 20

// Config contains exact, case-sensitive SQL Server table-name exclusions.
type Config struct {
	// Version identifies the supported configuration schema. It must be 1.
	Version int `yaml:"version"`
	// Exclude contains original SQL Server table names, before target-name mapping.
	Exclude []string `yaml:"exclude"`
}

// Load parses and strictly validates one YAML configuration file.
func Load(path string) (Config, error) {
	file, err := os.Open(path)
	if err != nil {
		return Config{}, fmt.Errorf("open tables config %q: %w", path, err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return Config{}, fmt.Errorf("inspect tables config %q: %w", path, err)
	}
	if info.Size() > maxConfigBytes {
		return Config{}, fmt.Errorf("tables config %q exceeds %d bytes", path, maxConfigBytes)
	}
	decoder := yaml.NewDecoder(file)
	decoder.KnownFields(true)
	var config Config
	if err := decoder.Decode(&config); err != nil {
		return Config{}, fmt.Errorf("decode tables config %q: %w", path, err)
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return Config{}, fmt.Errorf("tables config %q must contain one YAML document", path)
		}
		return Config{}, fmt.Errorf("decode trailing tables config %q: %w", path, err)
	}
	if err := config.Validate(); err != nil {
		return Config{}, fmt.Errorf("validate tables config %q: %w", path, err)
	}
	return config, nil
}

// Validate checks the schema version, exact identifiers and duplicate exclusions.
func (config Config) Validate() error {
	if config.Version != 1 {
		return fmt.Errorf("version must be 1, got %d", config.Version)
	}
	seen := make(map[string]struct{}, len(config.Exclude))
	for index, name := range config.Exclude {
		if err := ident.ValidateSource(name); err != nil {
			return fmt.Errorf("exclude[%d] %q: %w", index, name, err)
		}
		if _, exists := seen[name]; exists {
			return fmt.Errorf("duplicate exact exclusion %q", name)
		}
		seen[name] = struct{}{}
	}
	return nil
}
