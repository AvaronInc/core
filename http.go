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

type ResponseWriter struct {
	*http.Response
	*bytes.Buffer
}

func (rw ResponseWriter) Write(buf []byte) (int, error) {
	return rw.Buffer.Write(buf)
}

func (rw ResponseWriter) WriteHeader(status int) {
	rw.StatusCode = status
}

func (rw ResponseWriter) Header() http.Header {
	res := rw.Response
	if res.Header == nil {
		res.Header = make(http.Header)
	}
	return res.Header
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

func ServeHTTP(ctx context.Context) {
	var (
		err         error
		listener    net.Listener
		http, https chan net.Conn
		config      = &tls.Config{
			Certificates: make([]tls.Certificate, 1),
		}
	)
	config.Certificates[0], err = tls.LoadX509KeyPair("/etc/letsencrypt/live/isreal.estate/fullchain.pem",
		"/etc/letsencrypt/live/isreal.estate/privkey.pem")

	if err != nil {
		log.Println("failed to load certificates:", err)
	} else if listener, err := tls.Listen("tcp", ":8443", config); err != nil {
		log.Println("error starting HTTPS listener:", err)
	} else {
		https = make(chan net.Conn)
		go Listen(ctx, https, listener)
		log.Printf("listening on %s\n", listener.Addr().String())
	}

	if listener, err = net.Listen("tcp", ":8080"); err != nil {
		log.Fatalln("error starting HTTP listener:", err)
	} else {
		http = make(chan net.Conn)
		go Listen(ctx, http, listener)
		log.Printf("listening on %s\n", listener.Addr().String())
	}

	// load balancer - connection times out quicker the more connections there are
	const (
		total   = 256
		timeout = 60 * time.Second
	)

	var (
		tokens    = make(chan struct{})
		deadlines = make(chan time.Duration)
	)

	go func() {
		n := total
		for {
			// timeout*(total/n)
			// or
			// (timeout*total) / (timeout*n)
			d := (time.Duration(n+1) * timeout) / (time.Duration(total + 1))
			log.Printf("n: %d\n", n)
			if n > 0 {
				select {
				case tokens <- struct{}{}:
					n--
				case <-tokens:
					n++
				case deadlines <- d:
				case <-ctx.Done():
					return
				}
			} else {
				select {
				case <-tokens:
					n++
				case deadlines <- d:
				case <-ctx.Done():
					return
				}
			}

		}
	}()

	var (
		d    time.Duration
		conn net.Conn
		ok   bool
	)

	for {
		select {
		case conn, ok = <-http:
		case conn, ok = <-https:
		}

		if !ok {
			break
		}

		if d, ok = <-deadlines; !ok {
			break
		}

		log.Printf("duration: %20s\n", d)
		t := time.Now().Add(d)
		conn.SetReadDeadline(t)
		request, _ := context.WithDeadline(ctx, t)
		go func() {
			select {
			case <-tokens:
				// borrow token
			case <-request.Done():
				return
			}
			handle(request, conn, t)
			select {
			case tokens <- struct{}{}:
				// return token
			case <-request.Done():
				return
			}
		}()
	}
}

func handle(ctx context.Context, conn net.Conn, deadline time.Time) {
	var res = http.Response{
		Proto:      "HTTP/1.1",
		ProtoMajor: 1,
		ProtoMinor: 1,
		StatusCode: http.StatusOK,
	}

	reader := bufio.NewReader(conn)
	req, err := http.ReadRequest(reader)
	if err != nil {
		log.Println("error reading request:", err)
		goto end
	} else {
		res.Request = req
	}

	log.Printf("%50s %5s: %s\n", conn.RemoteAddr().String(), req.Method, req.URL.Path)

	switch req.URL.Path {
	case "/api/keys/ssh":
		if req.Method != "GET" {
			res.StatusCode = http.StatusMethodNotAllowed
			break
		}
		res.Body = io.NopCloser(strings.NewReader(PublicSSHKeys))
	case "/api/keys/wireguard":
		if req.Method != "GET" {
			res.StatusCode = http.StatusMethodNotAllowed
			break
		}
		buf, _ := PublicWireguardKey.MarshalText()
		res.Body = io.NopCloser(bytes.NewReader(buf))
	case "/api/link":
		if req.Method != "POST" {
			res.StatusCode = http.StatusMethodNotAllowed
			break
		}
		log.Printf("pairing with %s\n", conn.RemoteAddr().String())
		// check content-length
		if l := req.ContentLength; l < 44 || l > 44+1 {
			log.Printf("Request Content-Length (%d) != %d +/- 1/0\n", l, 44)
			res.StatusCode = http.StatusBadRequest
			break
		}

		// read body
		r := base64.NewDecoder(base64.StdEncoding, req.Body)

		var key Key
		_, err := io.ReadFull(r, key[:])
		if err != nil && err != io.EOF {
			log.Println("failed to public key:", err)
			res.StatusCode = http.StatusBadRequest
			break
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
				res.StatusCode = http.StatusInternalServerError
				break
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
				res.StatusCode = http.StatusUnauthorized
				break
			}
		}

		err = os.MkdirAll(fmt.Sprintf("pending/%s", key.String()), 0700)
		if err != nil {
			log.Println("failed to make pending link dir:", err)
			res.StatusCode = http.StatusInternalServerError
			break
		}
	case "/api/sdwan":
		if req.Method != "GET" {
			res.StatusCode = http.StatusMethodNotAllowed
			break
		}

		select {
		case <-ctx.Done():
		case RequestPeers <- conn:
		}

		return
	case "/api/logs":
		if req.Method != "GET" {
			res.StatusCode = http.StatusMethodNotAllowed
			break
		}
		var w io.WriteCloser
		res.Body, w = io.Pipe()
		select{
		case HealthCheckerRequests<-w:
		case <-ctx.Done():
			w.Close()
		}
		res.Header = http.Header{
			"Content-Type": []string{"application/json"},
		}
	case "/api/completions":
		if req.Method != "POST" {
			res.StatusCode = http.StatusMethodNotAllowed
			break
		}

		dec := json.NewDecoder(req.Body)
		if t, _ := dec.Token(); t != json.Delim('[') {
			log.Println("error reading services '[' for restart:", err)
			res.StatusCode = http.StatusInternalServerError
			break
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
			res.StatusCode = http.StatusInternalServerError
		}

		if err != nil {
			log.Println("error decoding message in chat request:", err)
			res.StatusCode = http.StatusBadRequest
			break
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
			res.StatusCode = http.StatusInternalServerError
			break
		}

		lres, err = llama.Client.Do(lreq)
		if err != nil {
			log.Println("error forwarding request to llama:", err)
			res.StatusCode = http.StatusInternalServerError
			break
		} else if lres.StatusCode < 200 || lres.StatusCode >= 300 {
			log.Println("error forwarding request to llama:", err)
			res.StatusCode = http.StatusInternalServerError
			break
		}

		var w io.WriteCloser
		res.Body, w = io.Pipe()
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

		res.Header = http.Header{
			"Content-Type": []string{"application/json"},
		}
	case "/api/services":
		if req.Method != "GET" {
			res.StatusCode = http.StatusMethodNotAllowed
			break
		}

		ctx, cancel := context.WithCancel(ctx)
		ch := make(chan Service)
		go func() {
			if err := ListServices(ctx, ch); err != nil {
				panic(err)
			}
		}()

		var w io.WriteCloser

		res.Body, w = io.Pipe()
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
		res.Header = http.Header{
			"Content-Type": []string{"application/json"},
		}
	case "/api/services/start", "/api/services/stop", "/api/services/restart":
		if req.Method != "POST" {
			res.StatusCode = http.StatusMethodNotAllowed
			break
		}
		var conn *systemd.Conn
		conn, err = systemd.NewSystemConnectionContext(ctx)
		if err != nil {
			log.Println("failed connecting to systemd:", err)
			res.StatusCode = http.StatusInternalServerError
			break
		}
		defer conn.Close()

		dec := json.NewDecoder(req.Body)
		if t, _ := dec.Token(); t != json.Delim('[') {
			log.Println("error reading services '[' for restart:", err)
			res.StatusCode = http.StatusInternalServerError
			break
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
			res.StatusCode = http.StatusInternalServerError
			break
		}

		if t, _ := dec.Token(); t != json.Delim(']') {
			log.Println("error reading services ']' for restart:", err)
			res.StatusCode = http.StatusInternalServerError
			break
		}

	default:
		if req.Method != "GET" {
			res.StatusCode = http.StatusNotFound
			break
		}
		rw := ResponseWriter{
			&res,
			bytes.NewBuffer(nil),
		}

		path := filepath.Clean(req.URL.Path)
		path = filepath.Join("/tmp/public/", path)
		if _, err := os.Stat(path); err != nil {
			path = "/tmp/public/"
		}
		log.Printf("serving file: %s\n", path)
		http.ServeFile(rw, req, path)
		res.Body = io.NopCloser(rw.Buffer)
	}

end:
	conn.SetWriteDeadline(deadline)
	defer conn.Close()
	if err != nil {
		log.Println("error processing request:", err)
	}
	res.Status = http.StatusText(res.StatusCode)

	err = res.Write(conn)
	if err != nil {
		log.Println("error writing request:", err)
		return
	}
}
