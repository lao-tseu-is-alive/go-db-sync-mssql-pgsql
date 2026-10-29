package syncmode

import "testing"

func TestParse(t *testing.T) {
	t.Parallel()
	for _, value := range []string{"replace", "merge"} {
		mode, err := Parse(value)
		if err != nil || string(mode) != value {
			t.Fatalf("Parse(%q) = %q, %v", value, mode, err)
		}
	}
	if _, err := Parse("append"); err == nil {
		t.Fatal("Parse accepted an unsupported mode")
	}
}
