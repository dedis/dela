package main

import (
	"os"
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestCommentLen(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), commentLenAnalyzer, "comment")
}

func TestIfCheck(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), ifInitAnalyzer, "ifcheck")
}

func TestMain(t *testing.T) {
	os.Args = []string{"", "help"}
	main()
}
