package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadDotEnvLoadsValuesWithoutOverridingEnvironment(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env")
	contents := strings.Join([]string{
		"# test dotenv",
		"DBSYNC_DOTENV_EXPORTED=from-file",
		"export DBSYNC_DOTENV_UNQUOTED=postgres://user:pass@host/db?x=1&y=2",
		"DBSYNC_DOTENV_SINGLE='literal value'",
		`DBSYNC_DOTENV_DOUBLE="line\nvalue"`,
	}, "\n")
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}

	setTemporaryEnvironment(t, "DBSYNC_DOTENV_EXPORTED", "from-environment")
	for _, name := range []string{"DBSYNC_DOTENV_UNQUOTED", "DBSYNC_DOTENV_SINGLE", "DBSYNC_DOTENV_DOUBLE"} {
		unsetTemporaryEnvironment(t, name)
	}
	loaded, err := LoadDotEnv(path)
	if err != nil || !loaded {
		t.Fatalf("LoadDotEnv loaded=%t err=%v", loaded, err)
	}
	wanted := map[string]string{
		"DBSYNC_DOTENV_EXPORTED": "from-environment",
		"DBSYNC_DOTENV_UNQUOTED": "postgres://user:pass@host/db?x=1&y=2",
		"DBSYNC_DOTENV_SINGLE":   "literal value",
		"DBSYNC_DOTENV_DOUBLE":   "line\nvalue",
	}
	for name, expected := range wanted {
		if actual := os.Getenv(name); actual != expected {
			t.Errorf("%s = %q, want %q", name, actual, expected)
		}
	}
}

func TestLoadDotEnvMissingFileIsOptional(t *testing.T) {
	t.Parallel()
	loaded, err := LoadDotEnv(filepath.Join(t.TempDir(), "missing.env"))
	if err != nil || loaded {
		t.Fatalf("LoadDotEnv loaded=%t err=%v", loaded, err)
	}
}

func TestLoadDotEnvRejectsMalformedInputWithoutEchoingIt(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(path, []byte("not-an-assignment-with-secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := LoadDotEnv(path)
	if err == nil {
		t.Fatal("LoadDotEnv accepted malformed input")
	}
	if strings.Contains(err.Error(), "secret") {
		t.Fatalf("parse error exposed line contents: %v", err)
	}
}

func setTemporaryEnvironment(t *testing.T, name, value string) {
	t.Helper()
	previous, existed := os.LookupEnv(name)
	if err := os.Setenv(name, value); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if existed {
			_ = os.Setenv(name, previous)
		} else {
			_ = os.Unsetenv(name)
		}
	})
}

func unsetTemporaryEnvironment(t *testing.T, name string) {
	t.Helper()
	previous, existed := os.LookupEnv(name)
	if err := os.Unsetenv(name); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if existed {
			_ = os.Setenv(name, previous)
		}
	})
}
