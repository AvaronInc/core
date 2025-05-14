package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strconv"
	systemd "github.com/coreos/go-systemd/v22/dbus"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	filepath "path"
	"strings"
	"time"
)

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
			fmt.Fprintf(os.Stderr, "error accepting connection: %+v\n", err)
			continue
		}
		select {
		case ch <- conn:
		case <-ctx.Done():
			return
		}
	}
}

type ChatRequest struct {
	prompt string
	id     int
	conn   net.Conn
}

type chat struct {
	words  chan string
	r      io.ReadCloser
	w      io.WriteCloser
	ctx    context.Context
	cancel context.CancelFunc
	cmd    *exec.Cmd
}


var (
	ChatRequests = make(chan ChatRequest)
	Chats = make([]*chat, 16)
)

func ServeChats(ctx context.Context) {
	var n, i int

	for {
		var req ChatRequest
		var res = http.Response{
			Proto:      "HTTP/1.1",
			ProtoMajor: 1,
			ProtoMinor: 1,
			StatusCode: http.StatusOK,
		}

		select {
		case req = <-ChatRequests:
			i = req.id
			if i < 0 && i >= len(Chats) {
				i = n % len(Chats)
			}
			if Chats[i] == nil {
				Chats[i] = &chat{}
				cmd := exec.Command("ollama", "run", "mixtral")
				var e1, e2 error
				Chats[i].w, e1 = cmd.StdinPipe()
				Chats[i].r, e2 = cmd.StdoutPipe()
				if e1 == nil && e2 != nil {
					e1 = e2
				}
				if e1 != nil {
					res.StatusCode = http.StatusInternalServerError
					fmt.Fprintf(os.Stderr, "failed to open pipes: %+v", e1)
					break
				}
				err := cmd.Start()
				if err != nil {
					res.StatusCode = http.StatusInternalServerError
					fmt.Fprintf(os.Stderr, "failed to start ollama: %+v", err)
					break
				}

				Chats[i].ctx, Chats[i].cancel = context.WithCancel(context.Background())
				go func(i int) {
					err := Chats[i].cmd.Wait()
					if err != nil {
						fmt.Fprintf(os.Stderr, "ollama exit error: %+v", err)
					}
					Chats[i].cancel()
				}(i)

				Chats[i].words = make(chan string)

				go func(i int) {
					var buf [32]byte
					defer close(Chats[i].words)
					for {
						n, err := Chats[i].r.Read(buf[:])
						if err != nil {
							break
						}
						select {
						case Chats[i].words <- string(buf[:n]):
						case <-Chats[i].ctx.Done():
							return
						}

					}
				}(i)

				n++
			}

			_, err := fmt.Fprintf(Chats[i].w, "%s\n", req.prompt)
			if err != nil {
				Chats[i%len(Chats)] = nil
				res.StatusCode = http.StatusInternalServerError
				fmt.Fprintf(os.Stderr, "failed to write to process: %+v", err)
				break
			}
			var w io.WriteCloser
			res.Body, w = io.Pipe()
			go func(i int) {
				defer w.Close()
				for {
					var word string
					select {
					case word = <-Chats[i].words:
					case <-time.After(2 * time.Second):
						return
					}
					fmt.Fprintf(w, "%s", word)
				}
			}(i)

		case <-ctx.Done():
			return
		}
		go func(conn net.Conn, res http.Response) {
			if err := res.Write(conn); err != nil {
				fmt.Fprintf(os.Stderr, "error writing request: %+v", err)
			}
			conn.Close()
		}(req.conn, res)

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
		fmt.Fprintf(os.Stderr, "failed to load certificates: %+v", err)
	} else if listener, err := tls.Listen("tcp", ":8443", config); err != nil {
		fmt.Fprintf(os.Stderr, "error starting HTTPS listener: %+v\n", err)
	} else {
		https = make(chan net.Conn)
		go Listen(ctx, https, listener)
		fmt.Fprintf(os.Stderr, "listening on %s\n", listener.Addr().String())
	}

	if listener, err = net.Listen("tcp", ":8080"); err != nil {
		fmt.Fprintf(os.Stderr, "error starting HTTP listener: %+v\n", err)
		os.Exit(1)
	} else {
		http = make(chan net.Conn)
		go Listen(ctx, http, listener)
		fmt.Fprintf(os.Stderr, "listening on %s\n", listener.Addr().String())
	}

	// load balancer - connection times out quicker the more connections there are
	const (
		total   = 256
		timeout = 10 * time.Second
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
			fmt.Fprintf(os.Stderr, "n: %d\n", n)
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

		fmt.Fprintf(os.Stderr, "duration: %20s\n", d)
		t := time.Now().Add(d)
		conn.SetDeadline(t)
		deadline, _ := context.WithDeadline(ctx, t)
		go func() {
			select {
			case <-tokens:
				// borrow token
			case <-deadline.Done():
				return
			}
			handle(deadline, conn)
			select {
			case tokens <- struct{}{}:
				// return token
			case <-ctx.Done():
			}
		}()
	}
}

func handle(ctx context.Context, conn net.Conn) {
	var res = http.Response{
		Proto:      "HTTP/1.1",
		ProtoMajor: 1,
		ProtoMinor: 1,
		StatusCode: http.StatusOK,
	}

	reader := bufio.NewReader(conn)
	req, err := http.ReadRequest(reader)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error reading request: %+v\n", err)
		goto end
	} else {
		res.Request = req
	}

	fmt.Fprintf(os.Stderr, "%50s %5s: %s\n", conn.RemoteAddr().String(), req.Method, req.URL.Path)

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
		fmt.Fprintf(os.Stderr, "pairing with %s\n", conn.RemoteAddr().String())
		// check content-length
		if l := req.ContentLength; l < 44 || l > 44+1 {
			fmt.Fprintf(os.Stderr, "Request Content-Length (%d) != %d +/- 1/0\n", l, 44)
			res.StatusCode = http.StatusBadRequest
			break
		}

		// read body
		r := base64.NewDecoder(base64.StdEncoding, req.Body)

		var key Key
		_, err := io.ReadFull(r, key[:])
		if err != nil && err != io.EOF {
			fmt.Fprintf(os.Stderr, "failed to public key: %+v\n", err)
			res.StatusCode = http.StatusBadRequest
			break
		}

		fmt.Fprintf(os.Stderr, "got buffer! %s\n", key.String())

		files, err := os.ReadDir("pending")
		if err == nil {
			// fine
		} else if os.IsNotExist(err) {
			// fine
			err := os.Mkdir("pending", 0700)
			if err != nil {
				fmt.Fprintf(os.Stderr, "failed to make 'pending' dir: %+v\n", err)
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
				fmt.Fprintf(os.Stderr, "case insensitive, matching pending link: %s & %s - rejecting & deleting\n", key.String(), files[i].Name())
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
			fmt.Fprintf(os.Stderr, "failed to make pending link dir: %+v\n", err)
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
	case "/api/chat":
		if req.Method != "POST" {
			res.StatusCode = http.StatusMethodNotAllowed
			break
		}
		var cr = ChatRequest{
			conn: conn,
		}

		params := req.URL.Query()
		if strs := params["id"]; len(strs) <= 0 {
			// ok
		} else if n, err := strconv.Atoi(strs[0]); err != nil {
			// ok
		} else if n < 0 || n >= len(Chats){
			fmt.Fprintf(os.Stderr, "chat id out of range", err)
			res.StatusCode = http.StatusBadRequest
			break
		} else {
			cr.id = n
		}

		buf, err := io.ReadAll(req.Body)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed reading body: %+v", err)
			res.StatusCode = http.StatusInternalServerError
			break
		}

		if len(buf) == 0 {
			fmt.Fprintf(os.Stderr, "empty body: %+v", err)
			res.StatusCode = http.StatusInternalServerError
			break
		}

		cr.prompt = string(buf)

		select {
		case <-ctx.Done():
		case ChatRequests <- cr:
			return
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
			fmt.Fprintf(os.Stderr, "failed connecting to systemd: %+v", err)
			res.StatusCode = http.StatusInternalServerError
			break
		}
		defer conn.Close()

		dec := json.NewDecoder(req.Body)
		if t, _ := dec.Token(); t != json.Delim('[') {
			fmt.Fprintf(os.Stderr, "error reading services '[' for restart: %+v", err)
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
			fmt.Fprintf(os.Stderr, "error reading services for restart: %+v", err)
			res.StatusCode = http.StatusInternalServerError
			break
		}

		if t, _ := dec.Token(); t != json.Delim(']') {
			fmt.Fprintf(os.Stderr, "error reading services ']' for restart: %+v", err)
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
		fmt.Fprintf(os.Stderr, "serving file: %s\n", path)
		http.ServeFile(rw, req, path)
		res.Body = io.NopCloser(rw.Buffer)
	}

end:

	defer conn.Close()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error processing request: %+v", err)
	}
	res.Status = http.StatusText(res.StatusCode)

	err = res.Write(conn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error writing request: %+v", err)
		return
	}
}
