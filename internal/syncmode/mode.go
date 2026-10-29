// Package syncmode defines the supported one-table PostgreSQL publication strategies.
package syncmode

import "fmt"

// Mode selects how a verified SQL Server snapshot replaces PostgreSQL data.
type Mode string

const (
	// Replace truncates the compatible target with RESTRICT and copies directly into it.
	Replace Mode = "replace"
	// Merge stages rows before reconciling the compatible target by primary key.
	Merge Mode = "merge"
)

// Parse validates a command-line synchronization mode.
func Parse(value string) (Mode, error) {
	mode := Mode(value)
	switch mode {
	case Replace, Merge:
		return mode, nil
	default:
		return "", fmt.Errorf("unsupported sync mode %q; choose replace or merge", value)
	}
}
