package llama

import (
	"context"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"time"
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
	if  host != "" {
		Client = http.Client{
			Transport: &http.Transport{
				DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
					return net.Dial("tcp", host)
				},
			},
		}
		return
	}

	ctx, cancel := context.WithCancel(context.Background())

	Client = http.Client{
		Transport: &http.Transport{
			DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
				return net.Dial("unix", "llama.sock")
			},
		},
	}
	os.Remove("llama.sock")
	llama := exec.CommandContext(ctx, "llama-server", "--host", "llama.sock", "--model", "mixtral.gguf")
	llama.Env = append(os.Environ(),
		"OLLAMA_NUM_GPU=999",
		"ZES_ENABLE_SYSMAN=1",
		"SYCL_CACHE_PERSISTENT=1",
		"OLLAMA_KEEP_ALIVE=10m",
		"SYCL_PI_LEVEL_ZERO_USE_IMMEDIATE_COMMANDLISTS=1")

	llama.Stdout = os.Stderr
	llama.Stderr = os.Stderr

	var err error
	if err = llama.Start(); err != nil {
		// TODO: log.Fatalln("failed to start llama server", err)
	}

	go func() {
		n := 0
		if err := llama.Wait(); err != nil {
			log.Println("llama server failed for some reason:", err)
			n = 1
		}
		return // TODO
		cancel()
		time.Sleep(5 * time.Second)
		os.Exit(n)
	}()
}
