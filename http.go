package main

import (
	"avaron/llama"
	network "avaron/net"
	"avaron/vertex"
	"avaron/health"
	wg "avaron/wireguard"
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
	"mime"
	"net"
	"net/http"
	"os"
	"os/exec"
	filepath "path"
	"sort"
	"strconv"
	"strings"
	"time"
)

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
		err      error
		listener net.Listener
		conns    chan net.Conn = make(chan net.Conn)
		config                 = &tls.Config{
			Certificates: make([]tls.Certificate, 1),
		}
	)

	if s := os.Getenv("SERVE_DIR"); s != "" {
		ServeDirectory = s
	} else {
		ServeDirectory = "public"
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
		timeout = 0 * time.Second
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
			var (
				d time.Duration
			)

			t := time.Now()
			if d := <-durations; d != 0 {
				conn.SetReadDeadline(t.Add(d))
			}

			reader := bufio.NewReader(conn)

			if req, err := http.ReadRequest(reader); err != nil {
				log.Println("error reading request:", err)
			} else {
				if d != 0 {
					var cancel context.CancelFunc
					ctx, cancel = context.WithDeadline(ctx, t)
					defer cancel()
				}
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

	i := func() int {
		var i, j int
		for i, j = 0, 0; i < len(req.URL.Path); i++ {
			if req.URL.Path[i] == '/' {
				j++
			}
			if j == 3 {
				break
			}
		}
		return i
	}()

	switch req.URL.Path[:i] {
	case "/api/keys":
		if req.Method != "GET" {
			return http.StatusMethodNotAllowed, nil, nil
		}

		switch req.URL.Path[i:] {
		case "/ssh":
			r = io.NopCloser(strings.NewReader(PublicSSHKeys))
		case "/wireguard":
			buf, _ := PublicWireguardKey.MarshalText()
			r = io.NopCloser(bytes.NewReader(buf))
		}
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

		var key vertex.Key
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
	case "/api/nodes":
		if req.Method != "GET" {
			return http.StatusMethodNotAllowed, nil, nil
		}
		var w io.WriteCloser
		r, w = io.Pipe()
		select {
		case RequestNodes <- w:
		case <-ctx.Done():
			w.Close()
		}

		header = http.Header{
			"Content-Type": []string{"application/json"},
		}
	case "/api/wireguard":
		switch req.Method {
		case "GET":
			info, err := wg.Interfaces(ctx)
			if err != nil {
				return http.StatusInternalServerError, nil, nil
			}
			log.Println("peer info", info)

			var w io.WriteCloser
			r, w = io.Pipe()
			enc := json.NewEncoder(w)
			go func() {
				defer w.Close()
				err := enc.Encode(info)
				if err != nil {
					log.Println("error encoding peers:", err)
				}
			}()

		case "POST":
			buf, err := io.ReadAll(req.Body)
			if err != nil {
				log.Println("failed reading body:", err)
				return http.StatusInternalServerError, nil, nil
			}

			var ip net.IP

			if len(buf) == 0 {
				// ok
				var routes map[string]*network.Route
				if routes, err = network.Routes(ctx); err != nil {
					log.Println("failed reading routes:", err)
					return http.StatusInternalServerError, nil, nil
				}

				var names []string
				for i := range routes {
					names = append(names, i)
				}

				log.Println("routes pre-sort", names)
				sort.Sort(&network.RouteMask{names, routes})
				log.Println("routes post-sort", names)
				if len(routes) < 1 {
					return http.StatusBadRequest, nil, nil
				}

				list, err := network.List(ctx)
				if err != nil {
					log.Println("failed to probe network links:", err)
					os.Exit(1)
				}

				route := routes[names[len(names)-1]]
				ip = func() net.IP {
					for _, link := range list {
						for _, info := range link.AddrInfo {
							if addr := net.ParseIP(info.Local); addr == nil {
								continue
							} else if route.Destination.Contains(addr) {
								return addr
							}
						}
					}
					return nil
				}()

				if ip == nil {
					log.Println("failed to to find IP matching first route:", route)
					os.Exit(1)
				}
			} else if ip = net.ParseIP(string(buf)); ip == nil {
				log.Println("failed parsing IP")
				return http.StatusBadRequest, nil, nil
			}

			public, private, err := wg.GenerateKeyPair()
			if err != nil {
				log.Println("error generating wireguard key pair:", err)
				return http.StatusInternalServerError, nil, nil
			}

			var pw io.WriteCloser

			qr := exec.Command("sh", "-c", "qrencode -t SVG | grep -v '<?' | grep -v '<!'")

			if pw, err = qr.StdinPipe(); err != nil {
				log.Println("failed spawning qrencode pipe:", err)
				return http.StatusInternalServerError, nil, nil
			}

			if r, err = qr.StdoutPipe(); err != nil {
				log.Println("failed spawning qrencode pipe:", err)
				return http.StatusInternalServerError, nil, nil
			}

			if err = qr.Start(); err != nil {
				log.Println("error generating wireguard key pair:", err)
				return http.StatusInternalServerError, nil, nil
			}

			dir := filepath.Join("peers", public.Path())
			if err := os.Mkdir(dir, 0700); err != nil {
				log.Println("error creating peer directory:", err)
				return http.StatusInternalServerError, nil, nil
			}

			fmt.Fprintf(pw, "[Interface]\n")
			fmt.Fprintf(pw, "Address = %s/32\n", public.GlobalAddress().IP.String())
			fmt.Fprintf(pw, "PrivateKey = %s\n", private.String())
			fmt.Fprintf(pw, "DNS = %s\n", public.GlobalAddress().IP.String())
			fmt.Fprintf(pw, "\n")

			fmt.Fprintf(pw, "[Peer]\n")
			fmt.Fprintf(pw, "PublicKey = %s\n", PublicWireguardKey.String())
			fmt.Fprintf(pw, "AllowedIPs = fc00:a7a0::/32\n")
			fmt.Fprintf(pw, "Endpoint = %s:%d\n", ip.String(), 51820)
			fmt.Fprintf(pw, "PersistentKeepalive = %d\n", 5)
			fmt.Fprintf(pw, "\n")
			pw.Close()

			cmd := exec.Command("sudo", "wg", "set", "avaron", "peer", public.String(), "allowed-ips", "fc00:a7a0::/32")
			buf, err = cmd.CombinedOutput()
			if err != nil {
				log.Println("failed adding peer", string(buf), err)
				return http.StatusInternalServerError, nil, nil

			}

		case "DELETE":
			buf, err := io.ReadAll(req.Body)
			if err != nil {
				log.Println("failed reading body:", err)
				return http.StatusInternalServerError, nil, nil
			}

			var key vertex.Key
			_, err = key.UnmarshalText(buf)
			if err != nil {
				log.Println("failed unmarshalling key:", err)
				return http.StatusInternalServerError, nil, nil
			}

			err = os.RemoveAll(filepath.Join("peers", key.Path()))
			if err != nil {
				log.Println("failed deleting peer:", err)
				return http.StatusNotFound, nil, nil
			}
			log.Println("deleted", key.Path())

			cmd := exec.Command("sudo", "wg", "set", "avaron", "peer", key.String(), "remove")
			buf, err = cmd.CombinedOutput()
			if err != nil {
				log.Println("failed removing peer", string(buf), err)
				return http.StatusInternalServerError, nil, nil

			}
		default:
			return http.StatusMethodNotAllowed, nil, nil
		}
		header = http.Header{
			"Content-Type": []string{"application/json"},
		}
	case "/api/shell":
		buf, err := io.ReadAll(req.Body)
		if err != nil {
			log.Println("failed reading body:", err)
			return http.StatusInternalServerError, nil, nil
		}
		log.Println("SHELL", string(buf))

		sh := exec.Command("sh", "-c", string(buf))

		var w io.WriteCloser
		r, w = io.Pipe()
		r = io.NopCloser(io.TeeReader(r, os.Stderr))
		sh.Stdout = w
		sh.Stderr = w

		if err := sh.Start(); err != nil {
			log.Println("failed starting shell", string(buf), err)
			return http.StatusInternalServerError, nil, nil
		}
		go func() {
			log.Println("command completed:", sh.Wait())
			w.Close()
		}()
	case "/api/health":
		if req.Method != "GET" {
			return http.StatusMethodNotAllowed, nil, nil
		}

		header = http.Header{
			"Content-Type": []string{"application/json"},
		}

		var w io.WriteCloser
		r, w = io.Pipe()

		switch req.URL.Path[i:] {
		case "/", "":


			enc := json.NewEncoder(w)
			go func() {
				defer w.Close()
				times := <-health.List
				if err := enc.Encode(times); err != nil {
					log.Println("error encoding:", err)
				}
			}()
		default:
			n, err := strconv.ParseInt(req.URL.Path[i+1:], 10, 64)
			if err != nil {
				log.Println("failed parsing integer", err)
				return http.StatusBadRequest, nil, nil
			}

			times := <-health.List

			if _, ok := times[n]; !ok {
				return http.StatusNotFound, nil, nil
			}
			health.Get<-health.Request{
				Time: n,
				WriteCloser: w,
			}
		}
	case "/api/completions":
		if req.Method != "POST" {
			return http.StatusMethodNotAllowed, nil, nil
		}

		req.RequestURI = ""
		req.URL.Scheme = "http"
		req.URL.Host = "localhost"
		req.URL.Path = "/completions"
		res, err := llama.Client.Do(req)
		if err != nil {
			log.Println("error forwarding request to llama:", err)
			return http.StatusInternalServerError, nil, nil
		} else if res.StatusCode < 200 || res.StatusCode >= 300 {
			log.Println("error forwarding request to llama:", err)
			return http.StatusInternalServerError, nil, nil
		}

		r = res.Body

		header = res.Header
	case "/api/services":
		if req.Method != "GET" {
			return http.StatusMethodNotAllowed, nil, nil
		}

		m, err := ListServices(ctx)
		if err != nil {
			panic(err)
		}

		buf, err := json.Marshal(m)
		if err != nil {
			log.Println("error marshalling systemd services", err)
			return http.StatusInternalServerError, nil, nil
		}

		r = io.NopCloser(bytes.NewReader(buf))
		header = http.Header{
			"Content-Type": []string{"application/json"},
		}
		return
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
		}

		info, err := os.Stat(path)
		if err != nil {
			return http.StatusNotFound, nil, nil
		} else if info.IsDir() {
			code = http.StatusMovedPermanently
			header = http.Header{
				"Location": []string{req.URL.Path + "/"},
			}
			return
		} else if ts := req.Header.Get("If-Modified-Since"); ts == "" {
			// fine
		} else if t, err := time.Parse(http.TimeFormat, ts); err != nil {
			// fine
			log.Printf("If-Modified-Since time parse failure: %v\n", err)
		} else if !info.ModTime().Truncate(time.Second).After(t) {
			return http.StatusNotModified, nil, nil
		}

		if r, err = os.Open(path); err != nil {
			log.Println("error openning:", err)
			return http.StatusInternalServerError, nil, nil
		}

		header = http.Header{
			"Content-Type":   []string{mime.TypeByExtension(filepath.Ext(path))},
			"Last-Modified":  []string{info.ModTime().UTC().Format(http.TimeFormat)},
			"Content-Length": []string{strconv.FormatInt(info.Size(), 10)},
		}
	}
	return
}
