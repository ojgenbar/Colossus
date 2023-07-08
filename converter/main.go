package main

import (
	"context"
	"errors"
	"github.com/confluentinc/confluent-kafka-go/v2/kafka"
	"github.com/ojgenbar/Colossus/converter/internal"
	"github.com/ojgenbar/Colossus/converter/utils"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

func Prepare() *utils.Config {
	_, ver := kafka.LibraryVersion()
	log.Printf("librdkafka version = %v", ver)

	cfg, err := utils.LoadConfig()
	if err != nil {
		log.Fatal(err)
	}
	internal.RegisterMetrics()
	return cfg
}

func main() {
	cfg := Prepare()

	var wg sync.WaitGroup
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go internal.StartProcessing(sigChan, &wg, cfg)
	wg.Add(1)

	server := &http.Server{Addr: cfg.Servers.System.Addr}

	http.Handle("/metrics", promhttp.Handler())

	go func() {
		if err := server.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("HTTP server error: %v", err)
		}
		log.Println("Stopped serving new connections.")
	}()

	// Wait until Consumer stops gracefully, then stop metrics server
	wg.Wait()

	shutdownCtx, shutdownRelease := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownRelease()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Fatalf("HTTP shutdown error: %v", err)
	}
	log.Println("Graceful HTTP server shutdown complete.")
}
