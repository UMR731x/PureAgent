package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"pureagent/internal/desktop"
	"pureagent/internal/studio"
	"pureagent/internal/update"
	"pureagent/web"
)

var version = "dev"

func main() {
	if len(os.Args) == 3 && os.Args[1] == update.HelperFlag {
		if err := update.RunHelper(os.Args[2]); err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		return
	}
	if err := run(); err != nil {
		if len(os.Args) == 3 && os.Args[1] == update.RestartFlag {
			// The installer needs this process to exit before it can restore the old client.
			fmt.Fprintln(os.Stderr, err)
		} else {
			desktop.ReportError(err)
		}
		os.Exit(1)
	}
}

func run() error {
	arguments := os.Args[1:]
	var restarted *update.Restart
	if len(arguments) == 2 && arguments[0] == update.RestartFlag {
		var err error
		restarted, err = update.ReadRestart(arguments[1])
		if err != nil {
			return err
		}
		arguments = restarted.Arguments
	}
	legacyPort := flag.Int("port", 57123, "deprecated; only used to recover old UI preferences, no port is opened")
	directory := flag.String("data", "data", "directory for configuration and sessions")
	if err := flag.CommandLine.Parse(arguments); err != nil {
		return err
	}
	dataSpecified := false
	flag.Visit(func(value *flag.Flag) { dataSpecified = dataSpecified || value.Name == "data" })
	if version != "dev" && restarted == nil && !dataSpecified {
		var err error
		*directory, err = desktop.DefaultDataDirectory()
		if err != nil {
			return err
		}
	}
	absolute, err := filepath.Abs(*directory)
	if err != nil {
		return err
	}
	release, running, err := desktop.AcquireWorkspace(absolute)
	if err != nil || running {
		return err
	}
	defer release()
	app, err := studio.New(absolute)
	if err != nil {
		return err
	}
	defer app.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	restartRequested := make(chan struct{})
	// Preserve the resolved directory even when a desktop launch used a different working directory.
	restartArguments := append([]string{}, arguments...)
	if !dataSpecified {
		restartArguments = append(restartArguments, "-data", app.Directory())
	}
	updater := update.New(ctx, version, restartArguments, update.Lifecycle{
		WaitForJobs: app.WaitForUpdate, ResumeJobs: app.ResumeAfterUpdate,
		Restart: func() { close(restartRequested) },
	})
	if restarted != nil && restarted.Failure != "" {
		updater.Failure(restarted.Failure)
	}
	return desktop.Run(ctx, desktop.Options{
		Directory: app.Directory(), Version: version, Restart: restartRequested,
		LegacyOrigin: fmt.Sprintf("http://127.0.0.1:%d/", *legacyPort),
		Resources:    app.Handler(web.Files, updater.Handler(), desktop.WorkspaceOrigin),
		Ready: func() error {
			if restarted != nil {
				return restarted.Ready(updater.Failure)
			}
			return nil
		},
	})
}
