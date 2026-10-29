// Package multisync plans and executes bounded multi-table snapshot refreshes.
package multisync

import (
	"fmt"
	"sort"

	"github.com/lao-tseu-is-alive/go-db-sync-mssql-pgsql/internal/ident"
	"github.com/lao-tseu-is-alive/go-db-sync-mssql-pgsql/internal/naming"
)

// Plan is the deterministic mapping of discovered source tables to targets.
type Plan struct {
	// SourceSchema is the discovered SQL Server schema.
	SourceSchema string
	// TargetSchema is the destination PostgreSQL schema.
	TargetSchema string
	// Tables contains every discovered base table, including exclusions.
	Tables []TablePlan
	// Collisions contains target names produced by multiple selected sources.
	Collisions []Collision
}

// TablePlan describes one discovered source table before database validation.
type TablePlan struct {
	// SourceName preserves the exact SQL Server table name.
	SourceName string
	// TargetName is the legacy-compatible PostgreSQL name.
	TargetName string
	// Excluded reports an exact configuration match on SourceName.
	Excluded bool
	// Collision reports that another selected source maps to TargetName.
	Collision bool
}

// Collision identifies selected sources that map to the same target table.
type Collision struct {
	// TargetName is the conflicting PostgreSQL table name.
	TargetName string
	// SourceNames contains the conflicting original names in deterministic order.
	SourceNames []string
}

// BuildPlan validates, sorts and maps discovered base tables without changing names first.
func BuildPlan(sourceSchema, targetSchema string, discovered, exclusions []string) (Plan, error) {
	if err := ident.ValidateSource(sourceSchema); err != nil {
		return Plan{}, fmt.Errorf("source schema: %w", err)
	}
	if err := ident.ValidateTarget(targetSchema); err != nil {
		return Plan{}, fmt.Errorf("target schema: %w", err)
	}
	discoveredSet := make(map[string]struct{}, len(discovered))
	for _, name := range discovered {
		if err := ident.ValidateSource(name); err != nil {
			return Plan{}, fmt.Errorf("discovered source table %q: %w", name, err)
		}
		if _, exists := discoveredSet[name]; exists {
			return Plan{}, fmt.Errorf("duplicate discovered source table %q", name)
		}
		discoveredSet[name] = struct{}{}
	}
	excludedSet := make(map[string]struct{}, len(exclusions))
	for _, name := range exclusions {
		if _, duplicate := excludedSet[name]; duplicate {
			return Plan{}, fmt.Errorf("duplicate exact exclusion %q", name)
		}
		if _, exists := discoveredSet[name]; !exists {
			return Plan{}, fmt.Errorf("exact exclusion %q does not match a discovered table", name)
		}
		excludedSet[name] = struct{}{}
	}
	names := append([]string(nil), discovered...)
	sort.Strings(names)
	plan := Plan{SourceSchema: sourceSchema, TargetSchema: targetSchema}
	targetSources := make(map[string][]string)
	for _, sourceName := range names {
		targetName := naming.TableName(sourceName)
		if err := ident.ValidateTarget(targetName); err != nil {
			return Plan{}, fmt.Errorf("map source table %q to target %q: %w", sourceName, targetName, err)
		}
		_, excluded := excludedSet[sourceName]
		plan.Tables = append(plan.Tables, TablePlan{SourceName: sourceName, TargetName: targetName, Excluded: excluded})
		if !excluded {
			targetSources[targetName] = append(targetSources[targetName], sourceName)
		}
	}
	for targetName, sourceNames := range targetSources {
		if len(sourceNames) < 2 {
			continue
		}
		sort.Strings(sourceNames)
		plan.Collisions = append(plan.Collisions, Collision{TargetName: targetName, SourceNames: sourceNames})
	}
	sort.Slice(plan.Collisions, func(left, right int) bool {
		return plan.Collisions[left].TargetName < plan.Collisions[right].TargetName
	})
	collidingTargets := make(map[string]struct{}, len(plan.Collisions))
	for _, collision := range plan.Collisions {
		collidingTargets[collision.TargetName] = struct{}{}
	}
	selected := 0
	for index := range plan.Tables {
		if plan.Tables[index].Excluded {
			continue
		}
		selected++
		_, plan.Tables[index].Collision = collidingTargets[plan.Tables[index].TargetName]
	}
	if selected == 0 {
		return Plan{}, fmt.Errorf("multi-table plan selects no tables from schema %q", sourceSchema)
	}
	return plan, nil
}

// Selected returns non-excluded tables in deterministic source-name order.
func (plan Plan) Selected() []TablePlan {
	selected := make([]TablePlan, 0, len(plan.Tables))
	for _, table := range plan.Tables {
		if !table.Excluded {
			selected = append(selected, table)
		}
	}
	return selected
}
