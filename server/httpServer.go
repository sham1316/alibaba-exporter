package server

import (
	"alibaba-exporter/config"
	"go.uber.org/dig"
)

type HttpServer interface {
	Start()
}

type HttpServerParams struct {
	dig.In
	config *config.Config
}

func NewHttpServer( /*p HttpServerParams*/ config *config.Config) HttpServer {
	return &ginServer{
		//HttpServerParams: p }
		config: config,
	}
}
