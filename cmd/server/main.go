package main

import (
	"github.com/gin-gonic/gin"
	"github.com/publicthrone547/towards_project/internal/config"
	"github.com/publicthrone547/towards_project/internal/routes"
)

func main() {
	r := gin.Default()
	cfg := config.Load()
	r.Use(func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Origin, Content-Type, Authorization")
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	})

	routes.Register(r)

	r.Run(":" + cfg.Port)
}