// Command doccheck enforces this repository's GoDoc and file-atlas contracts.
package main

import (
	"bytes"
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

var atlasEntryPattern = regexp.MustCompile(`(?m)^- ` + "`" + `([^` + "`" + `]+)` + "`" + ` — .+$`)
var versionPattern = regexp.MustCompile(`(?m)^\s*Version\s*=\s*"([^"]+)"`)

func main() {
	scope := flag.String("scope", "all", "check scope: go, atlas, or all")
	flag.Parse()
	if *scope != "go" && *scope != "atlas" && *scope != "all" {
		fmt.Fprintln(os.Stderr, "doccheck: scope must be go, atlas, or all")
		os.Exit(2)
	}
	var failures []string
	if *scope == "go" || *scope == "all" {
		failures = append(failures, checkGo()...)
	}
	if *scope == "atlas" || *scope == "all" {
		failures = append(failures, checkRepositoryAtlas()...)
	}
	if len(failures) > 0 {
		for _, failure := range failures {
			fmt.Fprintln(os.Stderr, "doccheck:", failure)
		}
		os.Exit(1)
	}
	fmt.Printf("doccheck: %s OK\n", *scope)
}

func inventory() ([]string, error) {
	command := exec.Command("git", "ls-files", "-z", "--cached", "--others", "--exclude-standard")
	output, err := command.Output()
	if err != nil {
		return nil, fmt.Errorf("inventory repository: %w", err)
	}
	parts := bytes.Split(bytes.TrimSuffix(output, []byte{0}), []byte{0})
	paths := make([]string, 0, len(parts))
	for _, part := range parts {
		if len(part) > 0 {
			paths = append(paths, string(part))
		}
	}
	sort.Strings(paths)
	return paths, nil
}

func checkRepositoryAtlas() []string {
	paths, err := inventory()
	if err != nil {
		return []string{err.Error()}
	}
	atlas, err := os.ReadFile("docs/atlas.md")
	if err != nil {
		return []string{fmt.Sprintf("read atlas: %v", err)}
	}
	versionSource, err := os.ReadFile("pkg/version/version.go")
	if err != nil {
		return []string{fmt.Sprintf("read version source: %v", err)}
	}
	match := versionPattern.FindSubmatch(versionSource)
	if len(match) != 2 {
		return []string{"cannot find Version constant"}
	}
	return checkAtlas(paths, string(atlas), string(match[1]))
}

func checkAtlas(inventoryPaths []string, atlas, version string) []string {
	var failures []string
	banner := "Tracked version: **v" + version + "**."
	if !strings.Contains(atlas, banner) {
		failures = append(failures, "atlas banner does not match "+banner)
	}
	counts := make(map[string]int)
	for _, match := range atlasEntryPattern.FindAllStringSubmatch(atlas, -1) {
		counts[match[1]]++
	}
	inventorySet := make(map[string]bool, len(inventoryPaths))
	for _, path := range inventoryPaths {
		inventorySet[path] = true
		if counts[path] == 0 {
			failures = append(failures, "atlas missing "+path)
		}
	}
	for path, count := range counts {
		if count > 1 {
			failures = append(failures, fmt.Sprintf("atlas duplicates %s (%d entries)", path, count))
		}
		if !inventorySet[path] {
			failures = append(failures, "atlas has stale path "+path)
		}
		if filepath.IsAbs(path) || filepath.Clean(path) != path {
			failures = append(failures, "atlas has non-canonical path "+path)
		}
	}
	sort.Strings(failures)
	return failures
}

func checkGo() []string {
	paths, err := inventory()
	if err != nil {
		return []string{err.Error()}
	}
	packages := make(map[string]bool)
	packageDocs := make(map[string]bool)
	var failures []string
	for _, path := range paths {
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			continue
		}
		source, err := os.ReadFile(path)
		if err != nil {
			failures = append(failures, fmt.Sprintf("%s: %v", path, err))
			continue
		}
		if bytes.Contains(source, []byte("Code generated")) {
			continue
		}
		fileFailures, packageName, documented := checkGoSource(path, source)
		failures = append(failures, fileFailures...)
		key := filepath.Dir(path) + ":" + packageName
		packages[key] = true
		packageDocs[key] = packageDocs[key] || documented
	}
	for key := range packages {
		if !packageDocs[key] {
			failures = append(failures, key+": missing purpose-driven package comment")
		}
	}
	sort.Strings(failures)
	return failures
}

func checkGoSource(path string, source []byte) ([]string, string, bool) {
	parsed, err := parser.ParseFile(token.NewFileSet(), path, source, parser.ParseComments)
	if err != nil {
		return []string{fmt.Sprintf("%s: parse: %v", path, err)}, "", false
	}
	documented := false
	if parsed.Doc != nil {
		text := strings.TrimSpace(parsed.Doc.Text())
		documented = strings.HasPrefix(text, "Package "+parsed.Name.Name+" ") ||
			(parsed.Name.Name == "main" && (strings.HasPrefix(text, "Command ") || strings.HasPrefix(text, "Package main ")))
	}
	var failures []string
	for _, declaration := range parsed.Decls {
		switch typed := declaration.(type) {
		case *ast.FuncDecl:
			if typed.Name.IsExported() && exportedReceiver(typed.Recv) && !commentStarts(typed.Doc, typed.Name.Name) {
				failures = append(failures, fmt.Sprintf("%s:%d: %s needs GoDoc beginning with its name", path, typed.Pos(), typed.Name.Name))
			}
		case *ast.GenDecl:
			for _, specification := range typed.Specs {
				failures = append(failures, checkSpec(path, typed, specification)...)
			}
		}
	}
	return failures, parsed.Name.Name, documented
}

func checkSpec(path string, declaration *ast.GenDecl, specification ast.Spec) []string {
	var failures []string
	switch typed := specification.(type) {
	case *ast.TypeSpec:
		if typed.Name.IsExported() && !commentStarts(firstComment(typed.Doc, declaration.Doc), typed.Name.Name) {
			failures = append(failures, fmt.Sprintf("%s: %s needs GoDoc beginning with its name", path, typed.Name.Name))
		}
		structure, ok := typed.Type.(*ast.StructType)
		if !ok || !typed.Name.IsExported() {
			return failures
		}
		for _, field := range structure.Fields.List {
			for _, name := range field.Names {
				if name.IsExported() && !commentStarts(firstComment(field.Doc, field.Comment), name.Name) {
					failures = append(failures, fmt.Sprintf("%s: field %s needs GoDoc beginning with its name", path, name.Name))
				}
			}
		}
	case *ast.ValueSpec:
		for _, name := range typed.Names {
			if name.IsExported() && !commentStarts(firstComment(typed.Doc, declaration.Doc), name.Name) {
				failures = append(failures, fmt.Sprintf("%s: %s needs GoDoc beginning with its name", path, name.Name))
			}
		}
	}
	return failures
}

func firstComment(comments ...*ast.CommentGroup) *ast.CommentGroup {
	for _, comment := range comments {
		if comment != nil {
			return comment
		}
	}
	return nil
}

func commentStarts(comment *ast.CommentGroup, name string) bool {
	return comment != nil && strings.HasPrefix(strings.TrimSpace(comment.Text()), name+" ")
}

func exportedReceiver(receiver *ast.FieldList) bool {
	if receiver == nil || len(receiver.List) == 0 {
		return true
	}
	expression := receiver.List[0].Type
	if pointer, ok := expression.(*ast.StarExpr); ok {
		expression = pointer.X
	}
	identifier, ok := expression.(*ast.Ident)
	return ok && identifier.IsExported()
}
