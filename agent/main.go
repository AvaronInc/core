package main

import (
	"bytes"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func main() {
	r := gin.Default()

	// Health endpoint
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status":  "healthy",
			"model":   "mistral:7b",
			"version": "1.0.0",
		})
	})

	// Prometheus metrics
	r.GET("/metrics", gin.WrapH(promhttp.Handler()))

	// LLM Query endpoint
	r.POST("/api/v1/agent/query", func(c *gin.Context) {
		var req struct {
			Prompt string `json:"prompt"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
			return
		}

		// Call Ollama HTTP API
		ollamaURL := "http://localhost:11434/api/generate"
		payload := []byte(`{"model":"mistral:7b","prompt":"` + req.Prompt + `"}`)
		resp, err := http.Post(ollamaURL, "application/json", bytes.NewBuffer(payload))
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Ollama not available"})
			return
		}
		defer resp.Body.Close()
		body, _ := ioutil.ReadAll(resp.Body)
		c.Data(resp.StatusCode, "application/json", body)
	})

	// Start server
	port := os.Getenv("PORT")
	if port == "" {
		port = "8000"
	}
	s := &http.Server{
		Addr:           ":" + port,
		Handler:        r,
		ReadTimeout:    10 * time.Second,
		WriteTimeout:   10 * time.Second,
		MaxHeaderBytes: 1 << 20,
	}
	log.Printf("Starting server on port %s...", port)
	s.ListenAndServe()
}
