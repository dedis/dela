package main

// This package provides a custom check for "go vet". The check verifies that no
// comments exceed the "MaxLen" length.
// It can be used like the following:
// `go build && go vet -vettool=./check -commentLen ./...`

import (
	"go/ast"
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
	for _, file := range pass.Files {
		ast.Inspect(file, func(node ast.Node) bool {
			switch x := node.(type) {
			case *ast.File:
				comments := x.Comments
				first := true
				for _, cg := range comments {
					for _, c := range cg.List {
						if first && strings.HasPrefix(c.Text, "// Code generated") {
							return false
						}
						lines := strings.Split(c.Text, "\n")
						for _, line := range lines {
							if len(line) > MaxLen {
								pass.Reportf(c.Pos(), "Comment too long: %s (%d)",
									line, len(line))
							}
						}
						first = false
					}
				}
				return false
			default:
			}
			return false
		})
	}
	return nil, nil
}
