package main

// This package provides a custom check for "go vet". The check verifies that no
// comments exceed the "MaxLen" length.
// It can be used like the following:
// `go build && go vet -vettool=./check -commentLen ./...`

import (
	"strings"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/unitchecker"
)

// MaxLen is the maximum length of a comment
var MaxLen = 80

var analyzer = &analysis.Analyzer{
	Name: "commentLen",
	Doc:  "checks the lengths of comments",
	Run:  run,
}

func main() {
	unitchecker.Main(
		analyzer,
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
				if len(c.Text) > MaxLen {
					pass.Reportf(c.Pos(), "Comment too long: %s (%d)",
						c.Text, len(c.Text))
				}
				isFirst = false
			}
		}
	}
	return nil, nil
}
