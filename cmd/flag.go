package cmd

import "time"

// StringFlag is a definition of a command flag expected to be parsed as a
// string.
type StringFlag struct {
	Name     string
	Usage    string
	Required bool
	Value    string
}

// Flag implements cmd.Flag.
func (flag StringFlag) Flag() {}

// DurationFlag is a definition of a command flag expected to be parsed as a
// duration.
type DurationFlag struct {
	Name     string
	Usage    string
	Required bool
	Value    time.Duration
}

// Flag implements cmd.Flag.
func (flag DurationFlag) Flag() {}
