package llama

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
)

type Message interface {
	String() string
	Type() string
}

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

/*
type Response struct {
	Body io.Reader
}
*/
/*
	{
	  "Prompt": "[INST] hello [/INST] ¡Hola! ¿Cómo estás? If you need help with a translation or have any questions about Spanish, I'm here to assist you. Go ahead and ask me anything.\n\nIf you want to start learning Spanish, there are many resources available online. Duolingo, Babbel, and Rosetta Stone are some popular language-learning\n[INST] hello hello [/INST] ",
	  "Format": null,
	  "Images": null,
	  "Options": {
	    "num_ctx": 4096,
	    "num_batch": 512,
	    "num_gpu": -1,
	    "num_keep": 4,
	    "seed": -1,
	    "num_predict": 81920,
	    "top_k": 40,
	    "top_p": 0.9,
	    "typical_p": 1,
	    "repeat_last_n": 64,
	    "temperature": 0.8,
	    "repeat_penalty": 1.1,
	    "stop": [
	      "[INST]",
	      "[/INST]"
	    ]
	  },
	  "Grammar": ""
	}
*/

var (
	Client = http.Client{
		Transport: &http.Transport{
			DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
				return net.Dial("unix", "llama.sock")
			},
		},
	}
)

func Do(ctx context.Context, messages <-chan Message, tokens chan<- []byte) error {
	defer close(tokens)

	prompt := &strings.Builder{}

	for m := range messages {
		switch m.Type() {
		case "user":
			fmt.Fprintf(prompt, "[INST]%s[/INST]", m.String())
		default:
			fmt.Fprintf(prompt, "%s", m.String())
		}
	}

	body := Request{
		Prompt: prompt.String(),
		Model:  "mixtral.gguf",
		Stream: true,
	}

	var (
		e1, e2 error
		req    *http.Request
		res    *http.Response
	)
	r, w := io.Pipe()

	req, e1 = http.NewRequestWithContext(ctx, "POST", "http://localhost/completions", r)
	if e1 != nil {
		return e1
	}

	enc := json.NewEncoder(w)

	go func() {
		defer w.Close()
		if e2 = enc.Encode(body); e2 != io.ErrClosedPipe && e2 != nil {
			log.Println("encoding error:", e2)
		}
	}()

	res, e1 = Client.Do(req)
	if e1 != nil {
		return e1
	}

	if e2 != nil {
		return e2
	}

	/*
		data: {"index":0,"content":" I","tokens":[315],"stop":false,"id_slot":-1,"tokens_predicted":152,"tokens_evaluated":9}

		data: {"index":0,"content":"'","tokens":[28742],"stop":false,"id_slot":-1,"tokens_predicted":153,"tokens_evaluated":9}

		data: {"index":0,"content":"m","tokens":[28719],"stop":false,"id_slot":-1,"tokens_predicted":154,"tokens_evaluated":9}

	*/

	var token Token

	scanner := bufio.NewScanner(res.Body)
	for scanner.Scan() {
		line := scanner.Bytes()
		switch {
		case len(line) == 0:
			continue
		case bytes.HasPrefix(line, []byte("data: ")):
			line = bytes.TrimPrefix(line, []byte("data: "))
			break
		default:
			log.Panicln("unexpected line from llama-server stream:", string(line))
		}
		if e1 = json.Unmarshal(line, &token); e1 != nil {
			return e1
		}
		select {
		case tokens <- []byte(token.Content):
		case <-ctx.Done():
			return nil
		}
	}

	return scanner.Err()
}
