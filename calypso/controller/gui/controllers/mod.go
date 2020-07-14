package controllers

import (
	"path/filepath"

	"go.dedis.ch/dela/calypso"
)

// NewCtrl creates a new Ctrl
func NewCtrl(path string, caly *calypso.Caly) *Ctrl {
	return &Ctrl{
		path: path,
		caly: caly,
	}
}

// Ctrl holds all the gui controllers. This struct allows us to share common
// data to all the controllers.
type Ctrl struct {
	path string
	caly *calypso.Caly
}

// abs is a utility to compute the absolute file path
func (c Ctrl) abs(path string) string {
	return filepath.Join(c.path, path)
}
