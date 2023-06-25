package main

import (
	"github.com/gin-gonic/gin"
	handlers "github.com/ojgenbar/Colossus/backend/internal"
	"github.com/ojgenbar/Colossus/backend/utils"
)

func Prepare() *utils.Config {
	cfg, err := utils.LoadConfig()
	if err != nil {
		panic(err)
	}
	handlers.PrepareS3Buckets(cfg.S3)
	return cfg
}

func ApiMiddleware(cfg *utils.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set("cfg", cfg)
		c.Next()
	}
}

func main() {
	cfg := Prepare()

	r := gin.Default()
	r.Use(ApiMiddleware(cfg))
	r.GET("/ping", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"message": "pong",
		})
	})

	r.POST("/upload-image", handlers.HandleFileUploadToBucket)
	r.GET("/retrieve-image/:type/:file", handlers.HandleFileRetrieveUploadToBucket)

	r.Run() // listen and serve on 0.0.0.0:8080 (for windows "localhost:8080")
}
