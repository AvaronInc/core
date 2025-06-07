package llama

import (
	"context"
	"log"
	"net"
	"net/http"
	"os"
)

type Request struct {
	Prompt string `json:"prompt"`
	Model  string `json:"model"`
	Stream bool   `json:"stream"`
}

type Token struct {
	Index     int    `json:"index"`
	Content   string `json:"content"`
	Tokens    []int  `json:"tokens"`
	Stop      bool   `json:"stop"`
	Slot      int    `json:"id_slot"`
	Predicted int    `json:"tokens_predicted"`
	Evaluated int    `json:"tokens_evaluated"`
}

var (
	Client http.Client
)

func Init() {
	host := os.Getenv("LLAMA_SERVER")
	log.Println("host", host)
	if host == "" {
		Client = http.Client{
			Transport: &http.Transport{
				DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
					return net.Dial("unix", "/var/run/llama.sock")
				},
			},
		}
	} else {
		Client = http.Client{
			Transport: &http.Transport{
				DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
					return net.Dial("tcp", host)
				},
			},
		}
	}
}
