package handlers

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/publicthrone547/towards_project/internal/ai"
	"github.com/publicthrone547/towards_project/internal/config"
)

type ImproveRequest struct {
	City        string                 `json:"city" binding:"required"`
	Date        string                 `json:"date,omitempty"`
	WeatherJSON map[string]interface{} `json:"weather,omitempty"`
}

type ImproveResponse struct {
	Suggestions string `json:"suggestions"`
}

func ImproveHandler(c *gin.Context) {
	var req ImproveRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "city required"})
		return
	}

	cfg := config.Load()
	key := cfg.GeminiAPIKey

	// request a short answer with header and word limit
	prompt := fmt.Sprintf("решение: Короткий ответ\nНе больше 50 слов. Provide practical, non-political, community-driven suggestions to improve the city '%s' (date=%s). Use the following metrics and propose infrastructure, environment, safety and public service improvements.\nMetrics:\n%v\n\nRespond concisely.", req.City, req.Date, req.WeatherJSON)

	reply, err := ai.AskGemini(key, "", prompt)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "ai failed", "detail": err.Error()})
		return
	}

	// enforce 50-word limit server-side as safety (truncate if model exceeded)
	words := splitWords(reply)
	if len(words) > 50 {
		words = words[:50]
		reply = joinWords(words)
	}

	c.JSON(http.StatusOK, ImproveResponse{Suggestions: reply})
}

// splitWords splits on whitespace
func splitWords(s string) []string {
	var out []string
	curr := ""
	for _, r := range s {
		if r == '\n' || r == '\t' || r == ' ' || r == '\r' {
			if curr != "" {
				out = append(out, curr)
				curr = ""
			}
		} else {
			curr += string(r)
		}
	}
	if curr != "" {
		out = append(out, curr)
	}
	return out
}

func joinWords(words []string) string {
	if len(words) == 0 {
		return ""
	}
	s := words[0]
	for i := 1; i < len(words); i++ {
		s += " " + words[i]
	}
	return s
}
