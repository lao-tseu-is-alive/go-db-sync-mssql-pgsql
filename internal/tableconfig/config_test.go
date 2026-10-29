package tableconfig

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadPreservesExactNames(t *testing.T) {
	path := writeConfig(t, "version: 1\nexclude:\n  - EmployeLoginLog\n  - XMLDocument\n")
	config, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"EmployeLoginLog", "XMLDocument"}
	for index := range want {
		if config.Exclude[index] != want[index] {
			t.Fatalf("Exclude[%d] = %q", index, config.Exclude[index])
		}
	}
}

func TestLoadRejectsUnknownFieldsAndDuplicates(t *testing.T) {
	for _, contents := range []string{
		"version: 1\nunknown: true\n",
		"version: 1\nexclude: [Document, Document]\n",
		"version: 2\nexclude: []\n",
		"version: 1\nexclude: ['']\n",
	} {
		if _, err := Load(writeConfig(t, contents)); err == nil {
			t.Fatalf("Load unexpectedly accepted %q", contents)
		}
	}
}

func TestRepositoryConfig(t *testing.T) {
	config, err := Load(filepath.Join("..", "..", "config", "config_Sample.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if config.Version != 1 {
		t.Fatalf("Version = %d", config.Version)
	}
}

func writeConfig(t *testing.T, contents string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "tables.yaml")
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadRejectsMultipleDocuments(t *testing.T) {
	_, err := Load(writeConfig(t, "version: 1\n---\nversion: 1\n"))
	if err == nil || !strings.Contains(err.Error(), "one YAML document") {
		t.Fatalf("Load error = %v", err)
	}
}
