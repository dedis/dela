package main

// This package provides a custom check for "go vet". The check verifies that no
// comments exceed the "MaxLen" length.
// It can be used like the following:
// `go build && go vet -vettool=./check -commentLen ./...`
// It ignores files that have as first comment a "// Code genereated..." comment
// it ignores comments that start with "//go:generate"

import (
	"strings"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/unitchecker"
)

// MaxLen is the maximum length of a comment
var MaxLen = 80

var commentAnalyzer = &analysis.Analyzer{
	Name: "commentLen",
	Doc:  "checks the lengths of comments",
	Run:  run,
}

func main() {
	unitchecker.Main(
		commentAnalyzer,
	)
}

// run parses all the comments in ast.File
func run(pass *analysis.Pass) (interface{}, error) {
fileLoop:
	for _, file := range pass.Files {
		isFirst := true
		for _, cg := range file.Comments {
			for _, c := range cg.List {
				if isFirst && strings.HasPrefix(c.Text, "// Code generated") {
					continue fileLoop
				}
				// in case of /* */ comment there might be multiple lines
				lines := strings.Split(c.Text, "\n")
				for _, line := range lines {
					if strings.HasPrefix(line, "//go:generate") {
						continue
					}
					if len(line) > MaxLen {
						pass.Reportf(c.Pos(), "Comment too long: %s (%d)",
							line, len(line))
					}
				}
				isFirst = false
			}
		}
	}
	return nil, nil
}
