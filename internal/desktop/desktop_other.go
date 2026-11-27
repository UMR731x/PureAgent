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

func AcquireWorkspace(string) (func(), bool, error) {
	return nil, false, fmt.Errorf("PureAgent 桌面客户端仅支持 Windows")
}

func DefaultDataDirectory() (string, error) { return "data", nil }

func ReportError(err error) { log.Print(err) }
