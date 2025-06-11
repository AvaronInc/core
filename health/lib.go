package health

import (
	"avaron/llama"
	"avaron/mickey"
	network "avaron/net"
	"bufio"
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os/exec"
	"strings"
	"time"
)

const HEALTH_PROMPT = `
You are a Linux Network Engineer. Your job is to diagnose network configurations.
If the information you're given is indicative of a HEALTHY network configuration, say HEALTHY.
If it's UNHEALTHY, say UNHEALTHY, followed by the command you'd like to run for further diagnostics.


Here is an example:
HEALTHY
Everything looks good


Here is another example:
UNHEALTHY

`+"```"+`
$ ip -br link show
`+"```"+`

Let's run "ip -br link show" to see more about the links


And another:
HEALTHY
docker0 is down. who gives a shit


And another:
UNHEALTHY
$ ping 8.8.8.8
there's not enough information here. let's try running "ping 8.8.8.8" to confirm that we're able to access the internet.

`

type Remark struct {
	User bool
	Content []byte
}

func Healthy(remarks []Remark) bool {
	for _, r := range remarks {
		if bytes.Contains(r.Content, []byte("UNHEALTHY")) {
			return false
		}
	}
	return true
}

func Split(p []byte) (r []Remark) {
	var i, j, k int
	for i = 0; i < len(p); {
		j = bytes.Index(p[i:], []byte("[INST]"))
		switch j {
		case -1:
			r = append(r, Remark{
				false,
				p[i:],
			})
			return
		case 0:
			j += i + len("[INST]")

			k = bytes.Index(p[j:], []byte("[/INST]"))
			if k <= 0 {
				return
			}

			k += j
			r = append(r, Remark{
				true,
				p[j:k],
			})
			i += k
		default:
			j += i

			r = append(r, Remark{
				false,
				p[i:j],
			})

			j += len("[INST]")

			k = bytes.Index(p[j:], []byte("[/INST]"))
			if k <= 0 {
				return
			}

			k += j
			r = append(r, Remark{
				true,
				p[j:k],
			})
			i += k
		}
	}
	return
}

func Tick(ctx context.Context, writer io.Writer) (err error) {
	r, w := io.Pipe()
	go func() {
		err = network.ListBrief(ctx, w)
	}()

	buf, _ := io.ReadAll(r)
	if err != nil {
		return
	}

	prompt := fmt.Sprintf("[INST]%s\nthe following is the output of ip -br addr show: %s[/INST]\n", HEALTH_PROMPT, string(buf))
	_, err = fmt.Fprintf(writer, "%s", prompt)
	if err != nil {
		return
	}

	for {
		buf, err = json.Marshal(llama.Request{
			Prompt: prompt,
			Model:  "mixtral.gguf",
			Stream: true,
		})

		if err != nil {
			log.Println("failed marshalling:", err)
			continue
		}

		var (
			req *http.Request
			res *http.Response
		)

		req, err = http.NewRequestWithContext(ctx, "POST", "http://localhost/completions", bytes.NewReader(buf))
		if err != nil {
			log.Println("error forming llama request:", err)
			break
		}

		res, err = llama.Client.Do(req)
		if err != nil {
			log.Println("error forwarding request to llama:", err)
			break
		} else if res.StatusCode < 200 || res.StatusCode >= 300 {
			log.Println("error forwarding request to llama:", err)
			break
		}

		r, w = io.Pipe()
		var token llama.Token
		scanner := bufio.NewScanner(res.Body)

		go func() {
			defer w.Close()
			for scanner.Scan() {
				line := scanner.Text()
				switch {
				case len(line) == 0:
					continue
				case strings.HasPrefix(line, "data: "):
					line = strings.TrimPrefix(line, "data: ")
					break
				default:
					log.Panicln("unexpected line from llama-server stream:", line)
				}
				if err = json.Unmarshal([]byte(line), &token); err != nil {
					log.Panicln("error marshalling llama json:", err, line)
				}
				_, err = w.Write([]byte(token.Content))
				if err != nil {
					log.Println("error writing llama token to response body:", err, line)
				}

			}
		}()

		buf, _ = io.ReadAll(io.TeeReader(r, writer))
		prompt += string(buf)

		var i int
		if i = strings.Index(string(buf), "\n$ "); i == -1 || strings.Index(string(buf), "UNHEALTHY") == -1 {
			break
		}

		shell := string(buf)[i+3:]
		if i = strings.Index(shell, "\n"); i != -1 {
			shell = shell[:i]
		}

		shell = strings.TrimSpace(shell)
		if shell == "" {
			break
		}
		log.Println("running suggested command:", shell)

		var out []byte
		out, err = exec.CommandContext(ctx, "/bin/sh", "-c", shell).CombinedOutput()
		if err != nil {
			log.Printf("failed to run ai suggestion: %+v\n", err)
			break
		}

		_, err = fmt.Fprintf(writer, "[INST]'%s':\n\n```\n%s\n```\n[/INST]", shell, string(out))
		if err != nil {
			return
		}

		prompt += fmt.Sprintf("[INST]%s[/INST]", out)
	}
	return
}

type Request struct {
	Time int64
	io.WriteCloser
}

var (
	Get  = make(chan Request)
	List = make(chan map[int64]bool)
)

func Loop(ctx context.Context) {
	ticker := time.NewTicker(time.Second)
	dialogues := make(map[time.Time]*mickey.Muxer)
	times := make(map[int64]bool)

	var (
		t  time.Time
		ch = make(chan *mickey.Muxer)
	)

	go func() {
		for _ = range ticker.C {
			log.Println("HealthChecker tick")
			r, w := io.Pipe()
			muxer := mickey.New(r)
			select {
			case ch <- muxer:
			case <-ctx.Done():
			}
			go io.Copy(io.Discard, muxer.NewReader())
			err := Tick(ctx, w)
			if err != nil {
				log.Println("HealthCheck error:", err)
			}
			w.Close()
		}
	}()

	for {
		select {
		case List<-times:
		case req := <-Get:
			m, ok := dialogues[time.Unix(req.Time, 0)]
			if !ok {
				log.Fatalln("expected to find time", req.Time)
			}

			go func(m *mickey.Muxer) {
				n, err := io.Copy(req.WriteCloser, m.NewReader())
				if err != nil {
					log.Println("error writing response:", err)
				}
				log.Println("copied", n)
				req.WriteCloser.Close()
			}(m)
		case m := <-ch:
			t = time.Now().Round(time.Second)
			dialogues[t] = m
			times = make(map[int64]bool)
			for t, m := range dialogues {
				b := false
				if m.EOF() {
					buf, _ := io.ReadAll(m.NewReader())
					b = Healthy(Split(buf))
				}

				times[t.Unix()] = b

			}
		case <-ctx.Done():
			return
		}
	}
}
