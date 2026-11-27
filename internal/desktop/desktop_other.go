//go:build !windows

package desktop

import (
	"context"
	"fmt"
	"log"
)

func Run(context.Context, Options) error {
	return fmt.Errorf("PureAgent 桌面客户端仅支持 Windows")
}

func Activate(string) bool { return false }

func DefaultDataDirectory() (string, error) { return "data", nil }

func ReportError(err error) { log.Print(err) }
