package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/publicthrone547/towards_project/internal/ai"
	"github.com/publicthrone547/towards_project/internal/config"
)

type AskRequest struct {
	Instruction string `json:"instruction,omitempty"`
	Prompt      string `json:"prompt" binding:"required"`
}

type AskResponse struct {
	Reply string `json:"reply"`
}

func AskHandler(c *gin.Context) {
	var req AskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "prompt required"})
		return
	}

	cfg := config.Load()
	key := cfg.GeminiAPIKey

	reply, err := ai.AskGemini(key, req.Instruction, req.Prompt)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "ai request failed", "detail": err.Error()})
		return
	}
	c.JSON(http.StatusOK, AskResponse{Reply: reply})
}
