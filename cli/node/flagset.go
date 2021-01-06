// This file contains the implementation of a simplified flag set used on the
// daemon side.
//
// Documentation Last Review: 13.10.2020
//

package node

import (
	"time"
)

// FlagSet is a serializable flag set implementation. It allows to pack the
// flags coming from a CLI application and send them to a daemon.
//
// - implements cli.Flags
type FlagSet map[string]interface{}

func (fset FlagSet) String(name string) string {
	switch v := fset[name].(type) {
	case string:
		return v
	default:
		return ""
	}
}

// StringSlice implements cli.Flags. It returns the slice of strings associated
// with the flag name if it is set, otherwise it returns nil.
func (fset FlagSet) StringSlice(name string) []string {
	switch v := fset[name].(type) {
	case []interface{}:
		values := make([]string, len(v))
		for i, str := range v {
			values[i] = str.(string)
		}

		return values
	default:
		return nil
	}
}

// Duration implements cli.Flags. It returns the duration associated with the
// flag name if it is set, otherwise it returns zero.
func (fset FlagSet) Duration(name string) time.Duration {
	switch v := fset[name].(type) {
	case float64:
		return time.Duration(v)
	default:
		return time.Duration(0)
	}
}

// Path implements cli.Flags. It returns the path associated with the flag name
// if it is set, otherwise it returns an empty string.
func (fset FlagSet) Path(name string) string {
	switch v := fset[name].(type) {
	case string:
		return v
	default:
		return ""
	}
}

// Int implements cli.Flags. It returns the integer associated with the flag if
// it is set, otherwise it returns zero.
func (fset FlagSet) Int(name string) int {
	switch v := fset[name].(type) {
	case int:
		return v
	case float64:
		// This is the case where flags have been JSON marshalled/unmarshalled.
		// In this case JSON uses the float type for numbers.
		if v == float64(int(v)) {
			return int(v)
		}

		return 0
	default:
		return 0
	}
}

// Bool implements cli.Flags. It return the boolean associated with the flag if
// it is set, otherwise it returns false.
func (fset FlagSet) Bool(name string) bool {
	switch v := fset[name].(type) {
	case bool:
		return v
	default:
		return false
	}
}
