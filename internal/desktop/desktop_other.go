//go:build !windows

package desktop

import "log"

func Start(Options) (func(), error) { return func() {}, nil }

func Activate(string) bool { return false }

func DefaultDataDirectory() (string, error) { return "data", nil }

func ReportError(err error) { log.Print(err) }
