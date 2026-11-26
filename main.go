package main

import (
	"context"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"

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
		desktop.ReportError(err)
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
	port := flag.Int("port", 57123, "local HTTP port")
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
	address := fmt.Sprintf("127.0.0.1:%d", *port)
	listener, err := net.Listen("tcp", address)
	if err != nil {
		if desktop.Activate("http://" + address) {
			return nil
		}
		return err
	}
	defer listener.Close()
	app, err := studio.New(*directory)
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
	server := &http.Server{
		Addr:        address,
		Handler:     app.Handler(web.Files, updater.Handler()),
		BaseContext: func(net.Listener) context.Context { return ctx },
	}
	server.RegisterOnShutdown(cancel)
	fmt.Printf("数据目录：%s\n", app.Directory())
	return serve(server, listener, app.Directory(), restarted, restartRequested, updater.Failure)
}

func serve(server *http.Server, listener net.Listener, directory string, restarted *update.Restart, restartRequested <-chan struct{}, reportUpdateFailure func(string)) error {
	finished := make(chan error, 1)
	go func() { finished <- server.Serve(listener) }()
	defer server.Close()
	quitRequested := make(chan struct{}, 1)
	stopDesktop, err := desktop.Start(desktop.Options{
		URL: "http://" + server.Addr, Directory: directory, Version: version, OpenBrowser: restarted == nil,
		Exit: func() {
			select {
			case quitRequested <- struct{}{}:
			default:
			}
		},
	})
	if err != nil {
		return err
	}
	defer stopDesktop()
	if restarted != nil {
		if err := restarted.Ready(reportUpdateFailure); err != nil {
			return err
		}
	}
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(signals)
	fmt.Printf("PureAgent %s · http://%s\n", version, server.Addr)
	select {
	case <-signals:
	case <-quitRequested:
	case <-restartRequested:
	case err := <-finished:
		return err
	}
	// Finish uploads and JSON writes before exiting. Update-triggered shutdown has already drained image jobs.
	return server.Shutdown(context.Background())
}
