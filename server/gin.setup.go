package server

import (
	"alibaba-exporter/config"
	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"net/http"
	"time"
)

type ginServer struct {
	config *config.Config
}

func (s *ginServer) Start() {
	router := gin.Default()

	router.GET(s.config.Http.RoutePrefix+"/metrics", gin.WrapH(promhttp.Handler()))

	//TODO
	router.GET(s.config.Http.RoutePrefix+"/liveness", func(context *gin.Context) {
		t := time.Now()
		context.JSON(http.StatusOK, gin.H{"responseTime": time.Since(t)})
	})
	//TODO
	router.GET(s.config.Http.RoutePrefix+"/readiness", func(context *gin.Context) {
		t := time.Now()
		context.JSON(http.StatusOK, gin.H{"responseTime": time.Since(t)})
	})

	router.NoRoute(func(context *gin.Context) {
		context.JSON(http.StatusNotFound, gin.H{"message": "Not found"})
	})

	go router.Run(":" + s.config.Http.Port)
	
}
