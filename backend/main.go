package main

import (
	"fmt"
	"github.com/gin-gonic/gin"
	handlers "github.com/ojgenbar/Colossus/backend/internal"
	"github.com/ojgenbar/Colossus/backend/utils"
	ginprometheus "github.com/zsais/go-gin-prometheus"
	"strings"
)

func Prepare() *utils.Config {
	cfg, err := utils.LoadConfig()
	if err != nil {
		panic(err)
	}
	handlers.PrepareS3Buckets(cfg.S3)
	handlers.RegisterMetrics()
	return cfg
}

func ApiMiddleware(cfg *utils.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set("cfg", cfg)
		c.Next()
	}
}

func PromMiddleware(c *gin.Context) string {
	url := c.Request.URL.Path
	for _, p := range c.Params {
		if p.Key == "type" || p.Key == "file" {
			url = strings.Replace(url, p.Value, ":"+p.Key, 1)
		}
	}
	return url
}

func main() {
	cfg := Prepare()

	p := ginprometheus.NewPrometheus("gin")
	p.ReqCntURLLabelMappingFn = PromMiddleware

	routerMain := gin.Default()
	p.Use(routerMain)

	routerMain.Use(ApiMiddleware(cfg))

	routerMain.GET("/healthz", handlers.HandleHealthz)
	routerMain.POST("/upload-image", handlers.HandleFileUploadToBucket)
	routerMain.GET("/retrieve-image/:type/:file", handlers.HandleFileRetrieveUploadToBucket)

	routerSystem := gin.Default()
	p.SetMetricsPath(routerSystem)

	fmt.Printf("Main server addr is %s.\n", cfg.Servers.Main.Addr)
	fmt.Printf("System server addr is %s.\n", cfg.Servers.System.Addr)
	go routerMain.Run(cfg.Servers.Main.Addr)
	routerSystem.Run(cfg.Servers.System.Addr)
}
