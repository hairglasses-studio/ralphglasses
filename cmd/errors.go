package cmd

import "errors"

// ErrChecksFailed is returned by RunE functions (doctor, validate, gatecheck,
// selftest) when one or more checks fail. Execute() maps it to os.Exit(1)
// so that cobra's error-handling pipeline runs first.
var ErrChecksFailed = errors.New("checks failed")
