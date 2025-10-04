package routes

import (
	"github.com/gin-gonic/gin"
	"github.com/publicthrone547/towards_project/internal/handlers"
)

func Register(r *gin.Engine) {
	r.GET("/", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})
	r.GET("/weather", handlers.GetWeather)
	r.POST("/ask", handlers.AskHandler)
}
