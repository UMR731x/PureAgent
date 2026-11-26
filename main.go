package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"

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
		log.Fatal(err)
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
	app, err := studio.New(*directory)
	if err != nil {
		return err
	}
	defer app.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	restartRequested := make(chan struct{})
	updater := update.New(ctx, version, arguments, update.Lifecycle{
		WaitForJobs: app.WaitForUpdate, ResumeJobs: app.ResumeAfterUpdate,
		Restart: func() { close(restartRequested) },
	})
	if restarted != nil && restarted.Failure != "" {
		updater.Failure(restarted.Failure)
	}
	server := &http.Server{
		Addr:    fmt.Sprintf("127.0.0.1:%d", *port),
		Handler: app.Handler(web.Files, updater.Handler()),
	}
	fmt.Printf("数据目录：%s\n", app.Directory())
	return serve(server, restarted, restartRequested, updater.Failure)
}

func serve(server *http.Server, restarted *update.Restart, restartRequested <-chan struct{}, reportUpdateFailure func(string)) error {
	listener, err := net.Listen("tcp", server.Addr)
	if err != nil {
		return err
	}
	defer listener.Close()
	finished := make(chan error, 1)
	go func() { finished <- server.Serve(listener) }()
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
	case <-restartRequested:
	case err := <-finished:
		return err
	}
	// Finish uploads and JSON writes before exiting. Update-triggered shutdown has already drained image jobs.
	return server.Shutdown(context.Background())
}
