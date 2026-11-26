package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"pureagent/internal/studio"
	"pureagent/web"
)

func main() {
	port := flag.Int("port", 57123, "local HTTP port")
	directory := flag.String("data", "data", "directory for configuration and sessions")
	flag.Parse()
	app, err := studio.New(*directory)
	if err != nil {
		log.Fatal(err)
	}
	server := &http.Server{
		Addr:    fmt.Sprintf("127.0.0.1:%d", *port),
		Handler: app.Handler(web.Files),
	}
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-signals
		app.Close()
		server.Close()
	}()
	fmt.Printf("PureAgent · http://%s\n数据目录：%s\n", server.Addr, app.Directory())
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}
