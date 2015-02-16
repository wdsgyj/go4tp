// cmd
package internal

import (
	"flag"
	"time"
)

var cmd = flag.NewFlagSet("tinypng", flag.ExitOnError)

func parse(args []string) error {
	return cmd.Parse(args)
}

func stringFlag(name string, value string, usage string) *string {
	return cmd.String(name, value, usage)
}

func float64Flag(name string, value float64, usage string) *float64 {
	return cmd.Float64(name, value, usage)
}

func boolFlag(name string, value bool, usage string) *bool {
	return cmd.Bool(name, value, usage)
}

func durationFlag(name string, value time.Duration, usage string) *time.Duration {
	return cmd.Duration(name, value, usage)
}

func intFlag(name string, value int, usage string) *int {
	return cmd.Int(name, value, usage)
}

func int64Flag(name string, value int64, usage string) *int64 {
	return cmd.Int64(name, value, usage)
}

func uintFlag(name string, value uint, usage string) *uint {
	return cmd.Uint(name, value, usage)
}

func uint64Flag(name string, value uint64, usage string) *uint64 {
	return cmd.Uint64(name, value, usage)
}
