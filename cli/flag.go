package cli

import "time"

// StringFlag is a definition of a command flag expected to be parsed as a
// string.
//
// - implements cli.Flag
type StringFlag struct {
	Name     string
	Usage    string
	Required bool
	Value    string
}

// Flag implements cli.Flag.
func (flag StringFlag) Flag() {}

// StringSliceFlag is a definition of a command flag expected to tbe parsed as a
// slice of strings.
//
// - implements cli.Flag
type StringSliceFlag struct {
	Name     string
	Usage    string
	Required bool
	Value    []string
}

// Flag implements cli.Flag.
func (flag StringSliceFlag) Flag() {}

// DurationFlag is a definition of a command flag expected to be parsed as a
// duration.
//
// - implements cli.Flag
type DurationFlag struct {
	Name     string
	Usage    string
	Required bool
	Value    time.Duration
}

// Flag implements cli.Flag.
func (flag DurationFlag) Flag() {}

// IntFlag is a definition of a command flag expected to be parsed as a integer.
//
// - implements cli.Flag
type IntFlag struct {
	Name     string
	Usage    string
	Required bool
	Value    int
}

// Flag implements cli.Flag.
func (flag IntFlag) Flag() {}

// BoolFlag is a definition of a command flag expected to be parsed as a
// boolean.
//
// - implements cli.Flag
type BoolFlag struct {
	Name     string
	Usage    string
	Required bool
	Value    bool
}

// Flag implements cli.Flag.
func (flag BoolFlag) Flag() {}
