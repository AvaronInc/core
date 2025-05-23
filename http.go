package main

import (
	"avaron/llama"
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"mime"
	systemd "github.com/coreos/go-systemd/v22/dbus"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	filepath "path"
	"strings"
	"time"
)

type Message struct {
	Content string `json:"content"`
	Role    string `json:"role"`
}

func Listen(ctx context.Context, ch chan net.Conn, listener net.Listener) {
	defer close(ch)
	defer listener.Close()

	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Println("error accepting connection:", err)
			continue
		}
		select {
		case ch <- conn:
		case <-ctx.Done():
			return
		}
	}
}

var (
	ServeDirectory string
)

func ServeHTTP(ctx context.Context) {
	var (
		err         error
		listener    net.Listener
		conns chan net.Conn = make(chan net.Conn )
		config      = &tls.Config{
			Certificates: make([]tls.Certificate, 1),
		}
	)

	if s := os.Getenv("SERVE_DIR"); s != "" {
		ServeDirectory = s
	} else {
		ServeDirectory = "/tmp/public"
	}

	config.Certificates[0], err = tls.LoadX509KeyPair("/etc/letsencrypt/live/isreal.estate/fullchain.pem",
		"/etc/letsencrypt/live/isreal.estate/privkey.pem")

	if err != nil {
		log.Println("failed to load certificates:", err)
	} else if listener, err := tls.Listen("tcp", ":8443", config); err != nil {
		log.Println("error starting HTTPS listener:", err)
	} else {
		go Listen(ctx, conns, listener)
		log.Printf("listening on %s\n", listener.Addr().String())
	}

	if listener, err = net.Listen("tcp", ":8080"); err != nil {
		log.Fatalln("error starting HTTP listener:", err)
	} else {
		go Listen(ctx, conns, listener)
		log.Printf("listening on %s\n", listener.Addr().String())
	}

	// load balancer - connection times out quicker the more connections there are
	const (
		total   = 256
		timeout = 60 * time.Second
	)

	var (
		tokens    = make(chan struct{})
		durations = make(chan time.Duration)
	)

	go func() {
		n := total
		for {
			// timeout*(total/n)
			// or
			// (timeout*total) / (timeout*n)
			d := (time.Duration(n+1) * timeout) / (time.Duration(total + 1))
			if n > 0 {
				select {
				case tokens <- struct{}{}:
					n--
				case <-tokens:
					n++
				case durations <- d:
				case <-ctx.Done():
					return
				}
			} else {
				select {
				case <-tokens:
					n++
				case durations <- d:
				case <-ctx.Done():
					return
				}
			}

		}
	}()

	for conn := range conns {
		select {
		case <-tokens:
			// borrow token
		case <-ctx.Done():
			return
		}
		go func(conn net.Conn) {
			defer conn.Close()

			t := time.Now().Add(<-durations)
			conn.SetReadDeadline(t)

			reader := bufio.NewReader(conn)

			if req, err := http.ReadRequest(reader); err != nil {
				log.Println("error reading request:", err)
			} else {
				ctx, cancel := context.WithDeadline(ctx, t)
				defer cancel()

				res := http.Response{
					Proto:      "HTTP/1.1",
					ProtoMajor: 1,
					ProtoMinor: 1,
				}
				res.StatusCode, res.Header, res.Body = handle(ctx, req, conn)
				if err != nil {
					log.Println("error processing request:", err)
				}

				res.Status = http.StatusText(res.StatusCode)

				log.Printf("%-24s %7s %-24s - %d(%s)\n", conn.RemoteAddr().String(), req.Method, req.URL.Path, res.StatusCode, res.Status)
				if err = res.Write(conn); err != nil {
					log.Println("error writing request:", err)
					return
				}
			}

			select {
			case tokens <- struct{}{}:
				// return token
			case <-ctx.Done():
				return
			}
		}(conn)
	}
}

func handle(ctx context.Context, req *http.Request, conn net.Conn) (code int, header http.Header, r io.ReadCloser) {
	var err error
	code = http.StatusOK

	switch req.URL.Path {
	case "/api/keys/ssh":
		if req.Method != "GET" {
			return http.StatusMethodNotAllowed, nil, nil
		}
		r = io.NopCloser(strings.NewReader(PublicSSHKeys))
	case "/api/keys/wireguard":
		if req.Method != "GET" {
			return http.StatusMethodNotAllowed, nil, nil
		}
		buf, _ := PublicWireguardKey.MarshalText()
		r = io.NopCloser(bytes.NewReader(buf))
	case "/api/link":
		if req.Method != "POST" {
			return http.StatusMethodNotAllowed, nil, nil
		}
		log.Printf("pairing with %s\n", conn.RemoteAddr().String())
		// check content-length
		if l := req.ContentLength; l < 44 || l > 44+1 {
			log.Printf("Request Content-Length (%d) != %d +/- 1/0\n", l, 44)
			return http.StatusBadRequest, nil, nil
		}

		// read body
		r := base64.NewDecoder(base64.StdEncoding, req.Body)

		var key Key
		_, err := io.ReadFull(r, key[:])
		if err != nil && err != io.EOF {
			log.Println("failed to public key:", err)
			return http.StatusBadRequest, nil, nil
		}

		log.Printf("got buffer! %s\n", key.String())

		files, err := os.ReadDir("pending")
		if err == nil {
			// fine
		} else if os.IsNotExist(err) {
			// fine
			err := os.Mkdir("pending", 0700)
			if err != nil {
				log.Println("failed to make 'pending' dir:", err)
				return http.StatusInternalServerError, nil, nil
			}
		} else if len(files) == 0 {
			// still fine
		} else {
			var i int
			var match bool
			for i = range files {
				if strings.EqualFold(files[i].Name(), key.String()) {
					match = true
					break
				}
			}

			if match {
				log.Printf("case insensitive, matching pending link: %s & %s - rejecting & deleting\n", key.String(), files[i].Name())
				err := os.Remove(filepath.Join("pending", files[i].Name()))
				if err != nil {
					// something nasty is going on
					panic(err)
				}
				return http.StatusUnauthorized, nil, nil
			}
		}

		err = os.MkdirAll(fmt.Sprintf("pending/%s", key.String()), 0700)
		if err != nil {
			log.Println("failed to make pending link dir:", err)
			return http.StatusInternalServerError, nil, nil
		}
	case "/api/sdwan":
		if req.Method != "GET" {
			return http.StatusMethodNotAllowed, nil, nil
		}
		select {
		case <-ctx.Done():
		case RequestPeers <- conn:
		}

		header = http.Header{
			"Content-Type": []string{"application/json"},
		}
	case "/api/logs":
		if req.Method != "GET" {
			return http.StatusMethodNotAllowed, nil, nil
		}
		var w io.WriteCloser
		r, w = io.Pipe()
		select{
		case HealthCheckerRequests<-w:
		case <-ctx.Done():
			w.Close()
		}
		header = http.Header{
			"Content-Type": []string{"application/json"},
		}
	case "/api/completions":
		if req.Method != "POST" {
			return http.StatusMethodNotAllowed, nil, nil
		}

		dec := json.NewDecoder(req.Body)
		if t, _ := dec.Token(); t != json.Delim('[') {
			log.Println("error reading services '[' for restart:", err)
			return http.StatusInternalServerError, nil, nil
		}

		prompt := &strings.Builder{}
		var m Message
		for dec.More() {
			if err = dec.Decode(&m); err != nil {
				break
			}
			switch m.Role {
			case "user":
				fmt.Fprintf(prompt, "[INST]%s[/INST]", m.Content)
			default:
				fmt.Fprintf(prompt, "%s", m.Content)
			}
		}

		if t, _ := dec.Token(); t != json.Delim(']') {
			err = fmt.Errorf("error reading services ']' for restart: %+v", err)
			return http.StatusInternalServerError, nil, nil
		}

		if err != nil {
			log.Println("error decoding message in chat request:", err)
			return http.StatusBadRequest, nil, nil
		}

		var buf []byte
		buf, err = json.Marshal(llama.Request{
			Prompt: prompt.String(),
			Model:  "mixtral.gguf",
			Stream: true,
		})

		var (
			lreq *http.Request
			lres *http.Response
		)

		lreq, err = http.NewRequestWithContext(ctx, "POST", "http://localhost/completions", bytes.NewReader(buf))
		if err != nil {
			log.Println("error forming llama request:", err)
			return http.StatusInternalServerError, nil, nil
		}

		lres, err = llama.Client.Do(lreq)
		if err != nil {
			log.Println("error forwarding request to llama:", err)
			return http.StatusInternalServerError, nil, nil
		} else if lres.StatusCode < 200 || lres.StatusCode >= 300 {
			log.Println("error forwarding request to llama:", err)
			return http.StatusInternalServerError, nil, nil
		}

		var w io.WriteCloser
		r, w = io.Pipe()
		var token llama.Token
		scanner := bufio.NewScanner(lres.Body)

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

		header = http.Header{
			"Content-Type": []string{"application/json"},
		}
	case "/api/services":
		if req.Method != "GET" {
			return http.StatusMethodNotAllowed, nil, nil
		}

		ctx, cancel := context.WithCancel(ctx)
		ch := make(chan Service)
		go func() {
			if err := ListServices(ctx, ch); err != nil {
				panic(err)
			}
		}()

		var w io.WriteCloser

		r, w = io.Pipe()
		go func() {
			enc := json.NewEncoder(w)

			err := func() (err error) {
				if _, err = fmt.Fprintf(w, "["); err != nil {
					return
				}
				i := 0
				for service := range ch {
					if i == 0 {
						// ok
					} else if _, err = fmt.Fprintf(w, ","); err != nil {
						return
					}

					if err = enc.Encode(service); err != nil {
						return
					}
					i += 1

				}
				if _, err = fmt.Fprintf(w, "]"); err != nil {
					return
				}
				return
			}()
			if err != nil {
				fmt.Fprintf(os.Stdout, "error encoding service: %+v\n", err)
			}
			cancel()
			w.Close()
		}()
		header = http.Header{
			"Content-Type": []string{"application/json"},
		}
	case "/api/services/start", "/api/services/stop", "/api/services/restart":
		if req.Method != "POST" {
			return http.StatusMethodNotAllowed, nil, nil
		}
		var conn *systemd.Conn
		conn, err = systemd.NewSystemConnectionContext(ctx)
		if err != nil {
			log.Println("failed connecting to systemd:", err)
			return http.StatusInternalServerError, nil, nil
		}
		defer conn.Close()

		dec := json.NewDecoder(req.Body)
		if t, _ := dec.Token(); t != json.Delim('[') {
			log.Println("error reading services '[' for restart:", err)
			return http.StatusInternalServerError, nil, nil
		}

		var service string
		for dec.More() {
			err = dec.Decode(&service)
			if err != nil {
				break
			}
			switch req.URL.Path {
			case "/api/services/start":
				_, err = conn.StartUnitContext(ctx, service, "replace", nil)
			case "/api/services/stop":
				_, err = conn.StopUnitContext(ctx, service, "replace", nil)
			case "/api/services/restart":
				_, err = conn.RestartUnitContext(ctx, service, "replace", nil)
			}
			if err != nil {
				break
			}
		}

		if err != nil {
			log.Println("error reading services for restart:", err)
			return http.StatusInternalServerError, nil, nil
		}

		if t, _ := dec.Token(); t != json.Delim(']') {
			log.Println("error reading services ']' for restart:", err)
			return http.StatusInternalServerError, nil, nil
		}
	case "", "/":
		code = http.StatusMovedPermanently
		header = http.Header{
			"Location": []string{"/dashboard/"},
		}
		return
	default:
		if req.Method != "GET" {
			return http.StatusNotFound, nil, nil
		}

		path := filepath.Join(ServeDirectory, filepath.Clean(req.URL.Path))

		if strings.HasSuffix(req.URL.Path, "/") {
			path = filepath.Join(ServeDirectory, filepath.Clean(req.URL.Path), "index.html")
		} else if info, err := os.Stat(path); err != nil {
			return http.StatusNotFound, nil, nil
		} else if info.IsDir() {
			code = http.StatusMovedPermanently
			header = http.Header{
				"Location": []string{req.URL.Path+"/"},
			}
			return
		}

		if r, err = os.Open(path); err != nil {
			log.Println("error openning:", err)
			return http.StatusInternalServerError, nil, nil
		}

		header = http.Header{
			"Content-Type": []string{mime.TypeByExtension(filepath.Ext(path))},
		}
	}
	return
}
