package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"time"
)

type LLMRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
	Stream bool   `json:"stream"`
}

type LLMResponse struct {
	Message struct {
		Content string `json:"content"`
	} `json:"message"`
}

func GenerateLLMResponse(prompt string) (string, error) {
	ollamaURL := "http://localhost:11434/api/generate"
	llmReq := LLMRequest{
		Model:  "mistral:7b",
		Prompt: prompt,
		Stream: false,
	}
	jsonData, _ := json.Marshal(llmReq)

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Post(ollamaURL, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var llmResp LLMResponse
	if err := json.NewDecoder(resp.Body).Decode(&llmResp); err != nil {
		return "", err
	}
	return llmResp.Message.Content, nil
}
