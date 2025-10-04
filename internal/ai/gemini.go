package ai

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"

	log "github.com/sirupsen/logrus"
)

type geminiResponse struct {
	Candidates []struct {
		Content struct {
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		} `json:"content"`
	} `json:"candidates"`
}

// Default embedded system instruction used when no instruction is provided.
var DefaultInstruction = `You are an AI assistant for a city improvement chat. Your goal is to help participants come up with ideas and provide advice on how to make the city better — improving quality of life, environment, infrastructure, safety, and public services. Respond in a friendly, clear, and constructive way. Encourage positive discussions, suggest practical solutions, global best practices, and modern technologies that can be applied locally. Avoid political topics or conflicts. Your main purpose is to inspire residents to collaborate and make their city a better place.`

func AskGemini(apiKey, instruction, promt string) (string, error) {
	if instruction == "" {
		instruction = DefaultInstruction
		log.Info("Using embedded default instruction")
	} else {
		log.Info("Using provided instruction")
	}
	url := "https://generativelanguage.googleapis.com/v1beta/models/gemini-2.0-flash:generateContent"

	reqBody := map[string]interface{}{
		"contents": []map[string]interface{}{
			{
				"parts": []map[string]string{
					{"text": instruction + "\n" + "не пиши что ты понял и т.п, переходи к делу\n" + promt},
				},
			},
		},
	}

	data, _ := json.Marshal(reqBody)

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(data))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-goog-api-key", apiKey)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	log.Infof("Gemini raw response: %s", string(body))

	var res struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}
	if err := json.Unmarshal(body, &res); err != nil {
		return "", err
	}

	if len(res.Candidates) > 0 && len(res.Candidates[0].Content.Parts) > 0 {
		return res.Candidates[0].Content.Parts[0].Text, nil
	}
	// fallback: return raw body as string so callers can see what the API returned
	return string(body), nil
}
