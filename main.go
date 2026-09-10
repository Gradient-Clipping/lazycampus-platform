// Lazy Campus is a free campus API edition of QuantumNous/new-api.
package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Gradient-Clipping/lazycampus-platform/campus"
)

var revision = "development"

func main() {
	config, err := campus.LoadConfig()
	if err != nil {
		log.Fatal(err)
	}
	if config.Revision == "development" {
		config.Revision = revision
	}
	app, err := campus.New(context.Background(), config)
	if err != nil {
		log.Fatal(err)
	}
	defer app.Close()
	server := &http.Server{Addr: config.Address, Handler: app.Router(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 100 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16 * 1024}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		deadline, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		_ = server.Shutdown(deadline)
	}()
	log.Printf("Lazy Campus platform started, revision=%s", config.Revision)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal("HTTP server failed")
	}
}
