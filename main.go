package main

import (
	"alibaba-exporter/alibaba"
	"alibaba-exporter/config"
	"alibaba-exporter/metrics"
	"alibaba-exporter/server"
	"context"
	"go.uber.org/dig"
	"go.uber.org/zap"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	container := dig.New()
	container.Provide(config.GetCfg)
	container.Provide(alibaba.NewAlibaba)
	container.Provide(metrics.NewCounters)
	container.Provide(server.NewHttpServer)

	ctx, cancelFunction := context.WithCancel(context.Background())
	defer func() {
		zap.S().Info("Main Defer: canceling context")
		cancelFunction()
		time.Sleep(time.Second * 5)
	}()

	if err := container.Invoke(func(alibaba alibaba.Alibaba) {
		go alibaba.MainLoop(ctx)
	}); err != nil {
		zap.S().Fatal(err)
	}

	if err := container.Invoke(func(httpServer server.HttpServer) {
		httpServer.Start()
	}); err != nil {
		zap.S().Fatal(err)
	}

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM, syscall.SIGINT)
	sigName := <-signals
	zap.S().Infof("Received SIGNAL - %s. Terminating...", sigName)
}
