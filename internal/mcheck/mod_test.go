package main

import (
	"os"
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestMain(m *testing.M) {
	os.Exit(m.Run())
}

func TestCommentLen(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), commentAnalyzer)
}
