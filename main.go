package main

import (
	network "avaron/net/linux"
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"math"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/user"
	filepath "path"
	"sort"
	"strconv"
	"strings"
	"syscall"
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

func truncate(f float64, precision int) float64 {
	shift := math.Pow(10, float64(precision))
	return math.Trunc(f*shift) / shift
}

// Location represents the geographical location of the branch
type Location struct {
	Latitude  float64 `json:"lat"`
	Longitude float64 `json:"lng"`
	Address   string  `json:"address"`
}

// Bandwidth represents the bandwidth details for a connection
type Bandwidth struct {
	Download int64 `json:"download"`
	Upload   int64 `json:"upload"`
}

// Connection represents a network connection
type Connection struct {
	ID        string    `json:"id"`
	Type      string    `json:"type"`
	Status    string    `json:"status"`
	Uptime    int64     `json:"uptime"`
	Bandwidth Bandwidth `json:"bandwidth"`
}

type Linker interface {
	ID() string
	Type() string
	Status() string
	Bandwidth() (int64, int64)
}

func AsConnection(l Linker) Connection {
	u, d := l.Bandwidth()
	return Connection{
		ID:     l.ID(),
		Type:   l.Type(),
		Status: l.Status(),
		Bandwidth: Bandwidth{
			Upload:   u,
			Download: d,
		},
	}
}

// Metrics represents various metrics related to the branch
type Metrics struct {
	Latency           int64   `json:"latency"`
	PacketLoss        float64 `json:"packetLoss"`
	Jitter            int64   `json:"jitter"`
	ActiveConnections int     `json:"activeConnections"`
}

// Branch represents the branch office information
type Branch struct {
	ID                  string     `json:"id"`
	Name                string     `json:"name"`
	Location            Location   `json:"location"`
	PrimaryConnection   Connection `json:"primaryConnection"`
	FailoverConnection1 Connection `json:"failoverConnection1"`
	IPAddress           string     `json:"ipAddress"`
	NetworkStatus       string     `json:"networkStatus"`
	Metrics             Metrics    `json:"metrics"`
}

func Analyze(links map[string]*network.Interface, metrics []network.TCPMetric) (total Metrics, oldest map[string]float64) {
	total = Metrics{
		ActiveConnections: len(metrics),
	}
	var sent, lost uint64
	oldest = make(map[string]float64)
	addresses := make(map[string]*network.Interface)
	for _, i := range links {
		sent += i.Stats64.Rx.Packets
		sent += i.Stats64.Tx.Packets

		lost += i.Stats64.Rx.Errors
		lost += i.Stats64.Tx.Errors

		lost += i.Stats64.Rx.Dropped
		lost += i.Stats64.Tx.Dropped

		lost += i.Stats64.Rx.OverErrors
		lost += i.Stats64.Tx.OverErrors
		for _, a := range i.AddrInfo {
			addresses[a.Local] = i
		}
	}
	var latency, jitter time.Duration
	for _, m := range metrics {
		latency += time.Duration(float64(time.Second) * m.RoundTripTime)
		jitter += time.Duration(float64(time.Second) * m.RoundTripTimeVariance)
		link, ok := addresses[m.Source.String()]
		if !ok {
			fmt.Fprintf(os.Stderr, "warning - failed to find link by address: %s\n", m.Source.String())
			continue
		}
		if m.Age > oldest[link.IfName] {
			oldest[link.IfName] = m.Age
		}
	}
	fmt.Println("abc", oldest)
	fmt.Println("abc", addresses)
	n := time.Duration(len(metrics))

	total.Latency = (latency / n).Milliseconds()
	total.Jitter = (jitter / n).Milliseconds()

	total.PacketLoss = truncate((float64(lost)/float64(sent))*100, 3)
	return
}

type IPer interface {
	IP() net.IP
}

/*
  {
    id: 'branch1',
    name: 'Branch Office 1',
    location: {
      lat: 37.7749,
      lng: -122.4194,
      address: '456 Market St, San Francisco, CA 94105'
    },
    primaryConnection: {
      id: 'primary-b1',
      type: 'fiber',
      status: 'degraded',
      uptime: 1296000, // 15 days in seconds
      bandwidth: {
        download: 500,
        upload: 500
      }
    },
    failoverConnection1: {
      id: 'failover1-b1',
      type: 'copper',
      status: 'active',
      uptime: 2592000,
      bandwidth: {
        download: 250,
        upload: 250
      }
    },
    ipAddress: '10.0.2.1',
    networkStatus: 'degraded',
    metrics: {
      latency: 35,
      packetLoss: 1.2,
      jitter: 4.5,
      activeConnections: 245
    }
  },
*/

func handle(conn net.Conn) {
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
		goto failure
	} else {
		res.Request = req
	}

	fmt.Fprintf(os.Stderr, "%s: %s\n", req.Method, req.URL.Path)

	switch req.URL.Path {
	case "/keys/ssh":
		if req.Method != "GET" {
			res.StatusCode = http.StatusMethodNotAllowed
			break
		}
		res.Body = io.NopCloser(strings.NewReader(PublicSSHKeys))
	case "/keys/wireguard":
		if req.Method != "GET" {
			res.StatusCode = http.StatusMethodNotAllowed
			break
		}
		res.Body = io.NopCloser(strings.NewReader(PublicWireguardKey))
	case "/link":
		if req.Method != "POST" {
			res.StatusCode = http.StatusMethodNotAllowed
			break
		}
		fmt.Fprintf(os.Stderr, "pairing with %s\n", conn.RemoteAddr().String())
		var (
			buf    [45]byte
			public [32]byte
		)

		// check content-length
		if a, b := req.ContentLength, int64(len(buf)); a < b || a > b+1 {
			fmt.Fprintf(os.Stderr, "Request Content-Length (%d) != %d +/- 1/0\n", a, b)
			res.StatusCode = http.StatusBadRequest
			break
		}

		// read body
		n, err := req.Body.Read(buf[:])
		if err != nil && err != io.EOF {
			fmt.Fprintf(os.Stderr, "failed to read body: %+v\n", err)
			res.StatusCode = http.StatusBadRequest
			break
		}

		// decode - to assert it's valid base64
		n, err = base64.StdEncoding.Decode(public[:], buf[:])
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to deocde body: %+v\n", err)
			res.StatusCode = http.StatusBadRequest
			break
		}

		// check decoded body length
		if n != len(public) {
			fmt.Fprintf(os.Stderr, "Decoded buffer len (%d) != %d\n", n, len(public))
			res.StatusCode = http.StatusBadRequest
		}
		fmt.Fprintf(os.Stderr, "got buffer! %s\n", string(buf[:]))

		base64.StdEncoding.Encode(buf[:], public[:])
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to re-encode body: %+v\n", err)
			res.StatusCode = http.StatusBadRequest
			break
		}

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
				if strings.EqualFold(files[i].Name(), string(buf[:])) {
					match = true
					break
				}
			}

			if match {
				fmt.Fprintf(os.Stderr, "case insensitive, matching pending link: %s & %s - rejecting & deleting\n", string(buf[:]), files[i].Name())
				err := os.Remove(filepath.Join("pending", files[i].Name()))
				if err != nil {
					// something nasty is going on
					panic(err)
				}
				res.StatusCode = http.StatusUnauthorized
				break
			}
		}

		err = os.MkdirAll(fmt.Sprintf("pending/%s", string(buf[:])), 0700)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to make pending link dir: %+v\n", err)
			res.StatusCode = http.StatusInternalServerError
			break
		}
	case "/sdwan":
		fmt.Fprintf(os.Stderr, "SDWAN\n")
		if req.Method != "GET" {
			res.StatusCode = http.StatusMethodNotAllowed
			break
		}

		hostname, err := os.Hostname()
		if err != nil {
			res.StatusCode = http.StatusInternalServerError
			err = fmt.Errorf("failed to query OS hostname: %+v", err)
			fmt.Fprintf(os.Stderr, "error processing request: %+v", err)
			break
		}

		links, err := network.List(context.Background())
		if err != nil {
			res.StatusCode = http.StatusInternalServerError
			err = fmt.Errorf("failed to query network links: %+v", err)
			fmt.Fprintf(os.Stderr, "error processing request: %+v", err)
			break
		}

		routes, err := network.Routes(context.Background())
		if err != nil {
			res.StatusCode = http.StatusInternalServerError
			err = fmt.Errorf("failed to query routes: %+v", err)
			fmt.Fprintf(os.Stderr, "error processing request: %+v", err)
			break
		}

		sort.Sort(network.RouteMask(routes))
		var connections []*network.Interface
		{
			m := make(map[string]struct{})
			for _, r := range routes {
				_, ok := m[r.Device]
				if ok {
					continue
				}
				link, ok := links[r.Device]
				if !ok {
					fmt.Fprintf(os.Stderr, "warning - failed to find link %s given device name from routing table", r.Device)
					continue
				}
				m[r.Device] = struct{}{}
				connections = append(connections, link)
			}
		}

		tcpmetrics, err := network.Metrics(context.Background())
		if err != nil {
			res.StatusCode = http.StatusInternalServerError
			err = fmt.Errorf("failed to query network metrics: %+v", err)
			fmt.Fprintf(os.Stderr, "error processing request: %+v", err)
			break
		}

		metrics, ages := Analyze(links, tcpmetrics)
		branch := Branch{
			ID:            string(PublicWireguardKey),
			Name:          hostname,
			NetworkStatus: "degraded",
			Metrics:       metrics,
		}

		if len(connections) > 1 {
			c := AsConnection(connections[0])
			c.Uptime = int64(ages[connections[0].IfName])
			branch.PrimaryConnection = c
			if addrs := connections[0].AddrInfo; len(addrs) > 0 {
				sort.Sort(network.AddressMask(addrs))
				branch.IPAddress = addrs[0].Local
			}
			branch.NetworkStatus = c.Status
		}

		if len(connections) > 2 {
			c := AsConnection(connections[1])
			c.Uptime = int64(ages[connections[1].IfName])
			branch.FailoverConnection1 = c
		}

		buf, err := json.Marshal([]Branch{branch})
		if err != nil {
			res.StatusCode = http.StatusInternalServerError
			err = fmt.Errorf("failed to marshal: %+v", err)
			break
		}
		fmt.Fprintf(os.Stderr, "writing: %s\n", string(buf))
		res.Body = io.NopCloser(bytes.NewReader(buf))
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
		fmt.Fprintf(os.Stderr, "serving file: %s\n", path)
		http.ServeFile(rw, req, path)
		res.Body = io.NopCloser(rw.Buffer)
	}

failure:
	if err != nil {
		fmt.Fprintf(os.Stderr, "error processing request: %+v", err)
		return
	}
	res.Status = http.StatusText(res.StatusCode)

	err = res.Write(conn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error writing request: %+v", err)
		return
	}
}

var (
	Home               fs.FS
	PublicSSHKeys      string
	PublicWireguardKey string
)

func controller() error {
	if len(os.Args) <= 1 {
		return fmt.Errorf("controller invoked with no arguments")
	}

	switch os.Args[1] {
	case "address":
		if len(os.Args) <= 2 {
			return fmt.Errorf("not enough arguments")
		}
		ip, network, err := net.ParseCIDR(os.Args[2])
		if err != nil {
			return fmt.Errorf("error parsing address: %+v", err)
		}

		i := strings.Index(network.String(), "/")
		if i == -1 {
			return fmt.Errorf("/ not found in generated IP address string")
		}

		address := fmt.Sprintf("%s\n", ip.String())
		mask := fmt.Sprintf("%s\n", network.String()[i+1:])

		err = os.WriteFile("address", []byte(address), 0644)
		if err != nil {
			return fmt.Errorf("failed writing address file: %+v", err)
		}
		err = os.WriteFile("mask", []byte(mask), 0644)
		if err != nil {
			return fmt.Errorf("failed writing mask file: %+v", err)
		}
	case "link":
		if len(os.Args) <= 2 {
			return fmt.Errorf("not enough arguments")
		}
		link := fmt.Sprintf("http://%s/keys/wireguard", os.Args[2])
		r1, err := http.Get(link)
		if err != nil {
			return err
		}
		defer r1.Body.Close()

		if r1.StatusCode != http.StatusOK {
			return fmt.Errorf("%s responded with %s", link, r1.Status)
		}

		wireguard, err := io.ReadAll(r1.Body)
		if err != nil {
			return fmt.Errorf("reading response body: %+v", err)
		}

		link = fmt.Sprintf("http://%s/keys/ssh", os.Args[2])
		r2, err := http.Get(link)
		if err != nil {
			return err
		}
		defer r2.Body.Close()

		if r2.StatusCode != http.StatusOK {
			return fmt.Errorf("%s responded with %s", link, r2.Status)
		}

		ssh, err := io.ReadAll(r2.Body)
		if err != nil {
			return fmt.Errorf("reading response body: %+v", err)
		}

		wireguard = bytes.TrimSpace(wireguard)
		dir := fmt.Sprintf("links/%s", strings.Replace(string(wireguard), "/", "-", -1))
		err = os.Mkdir(dir, 0755)
		if err != nil {
			return fmt.Errorf("reading response body: %+v", err)
		}

		// TODO: rm -rf on failure
		url, _ := url.Parse(link)
		host := url.Host
		if i := strings.Index(host, ":"); i != 0 {
			host = host[:i]
		}
		host = fmt.Sprintf("%s\n", host)
		err = os.WriteFile(fmt.Sprintf("%s/host", dir), []byte(host), 0555)
		if err != nil {
			return fmt.Errorf("failed writing host '%s' to file: %+v", url.Host, err)
		}

		err = os.WriteFile(fmt.Sprintf("%s/ssh", dir), ssh, 0555)
		if err != nil {
			return fmt.Errorf("failed writing ssh file: %+v", err)
		}

		fmt.Fprintf(os.Stderr, "got public keys - wg: %s, ssh: %s\n", string(wireguard), string(ssh))
		err = initVPN()
		if err != nil {
			return err
		}
	default:
		return fmt.Errorf("unknown option: %s", os.Args[1])
	}

	return nil
}

func initVPN() error {
	cmd := exec.Command(".local/bin/init.sh")
	output, err := cmd.CombinedOutput()
	if err != nil {
		err = fmt.Errorf("error from init.sh: %+v - output: %s", err, string(output))
	}
	return err
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

func Serve(ctx context.Context) {
	cert, err := tls.LoadX509KeyPair("/etc/letsencrypt/live/isreal.estate/fullchain.pem",
		"/etc/letsencrypt/live/isreal.estate/privkey.pem")

	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to load certificates: %+v", err)
		os.Exit(1)
	}

	config := &tls.Config{Certificates: []tls.Certificate{cert}}

	listener, err := tls.Listen("tcp", ":8443", config)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error starting HTTPS listener: %+v\n", err)
		return
	}

	https := make(chan net.Conn)
	go Listen(ctx, https, listener)

	fmt.Fprintf(os.Stderr, "listening on %s\n", listener.Addr().String())

	listener, err = net.Listen("tcp", ":8080")
	if err != nil {
		fmt.Fprintf(os.Stderr, "error starting HTTP listener: %+v\n", err)
		return
	}

	http := make(chan net.Conn)
	go Listen(ctx, http, listener)
	fmt.Fprintf(os.Stderr, "listening on %s\n", listener.Addr().String())

	// load balancer - connection times out quicker the more connections there are
	const (
		total   = 256
		timeout = 10 * time.Second
	)

	var (
		tokens    = make(chan struct{})
		deadlines = make(chan time.Time)
	)

	go func() {
		n := total
		for {
			// timeout*(total/n)
			// or
			// (timeout*total) / (timeout*n)
			d := (time.Duration(n+1) * timeout) / (time.Duration(total + 1))
			t := time.Now().Add(d)
			fmt.Fprintf(os.Stderr, "n: %d\n", n)
			if n > 0 {
				select {
				case tokens <- struct{}{}:
					n--
				case <-tokens:
					n++
				case deadlines <- t:
				case <-ctx.Done():
					return
				}
			} else {
				select {
				case <-tokens:
					n++
				case deadlines <- t:
				case <-ctx.Done():
					return
				}
			}

		}
	}()

	var (
		d    time.Time
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

		fmt.Fprintf(os.Stderr, "duration: %20s\n", d.Sub(time.Now()))
		conn.SetDeadline(d)
		deadline, cancel := context.WithDeadline(ctx, d)
		go func() {
			defer cancel()
			select {
			case <-tokens:
				// borrow token
				cancel()
			case <-deadline.Done():
				return
			}
			handle(conn)
			conn.Close()
			select {
			case tokens <- struct{}{}:
				// return token
			case <-ctx.Done():
			}
		}()
	}
}

func main() {
	if len(os.Args) < 1 {
		fmt.Fprintf(os.Stderr, "unnamed binary")
		os.Exit(1)
	}

	// portability hack
	if len(os.Args) > 1 && os.Args[1] == "netmask" {
		if len(os.Args) <= 2 {
			fmt.Fprintf(os.Stderr, "netmask requires CIDR address\n")
			os.Exit(1)
		}
		_, net, err := net.ParseCIDR(os.Args[2])
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to parse CIDR: %+v\n", err)
			os.Exit(1)
		}
		fmt.Fprintf(os.Stdout, "%s\n", net.String())
		os.Exit(1)
	}

	base := filepath.Base(os.Args[0])
	user, err := user.Lookup(base)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to find user '%s' - %+v\n", base, err)
		os.Exit(1)
	}

	uid, err := strconv.Atoi(user.Uid)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to parse %s's UID '%s' - %+v\n", base, user.Uid, err)
		os.Exit(1)
	}

	puid := os.Getuid()
	if puid != 0 && puid != uid {
		fmt.Fprintf(os.Stderr, "%s was invoked by UID %d, however, the %s user has UID %d\n", os.Args[0], puid, filepath.Base(os.Args[0]), uid)
		os.Exit(1)
	}

	if err := os.Chdir(user.HomeDir); err != nil {
		fmt.Fprintf(os.Stderr, "changing to home directory - %+v\n", err)
		os.Exit(1)
	}

	createPIDFile := func() {
		buf := fmt.Sprintf("%d\n", os.Getpid())
		err := os.WriteFile("pid", []byte(buf), 0644)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to create PID file: %+v\n", err)
			os.Exit(1)
		}
	}

	buf, err := os.ReadFile("pid")
	if err != nil && os.IsNotExist(err) {
		if len(os.Args) > 1 {
			fmt.Fprintf(os.Stderr, "attempted to invoking controller without existing process\n")
			os.Exit(1)
		}
		createPIDFile()
	} else if err != nil {
		fmt.Fprintf(os.Stderr, "failed to read PID file: %+v\n", err)
		os.Exit(1)
	} else if pid, err := strconv.Atoi(string(bytes.TrimSpace(buf))); err != nil {
		fmt.Fprintf(os.Stderr, "failed to read PID file: %+v\n", err)
		os.Exit(1)
	} else {
		var e1, e2 error
		var proc *os.Process
		proc, e1 = os.FindProcess(pid)
		if e1 == nil {
			e2 = proc.Signal(syscall.Signal(0))
		}
		if e1 == nil && e2 == nil {
			// controller mode
			err := controller()
			if err != nil {
				fmt.Fprintf(os.Stderr, "%+v\n", err)
				os.Exit(1)
			}
			os.Exit(0)
		} else {
			createPIDFile()
		}
	}

	// reading all SSH public keys
	ssh := os.DirFS(".ssh")

	paths, err := fs.Glob(ssh, "*.pub")
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to find public SSH keys: %+v\n", err)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "found %d SSH public keys: %s\n", len(paths), strings.Join(paths, ", "))

	for _, path := range paths {
		pub, err := fs.ReadFile(ssh, path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error reading %s: %+v\n", path, err)
			continue
		}
		PublicSSHKeys += string(pub)
	}

	if len(PublicSSHKeys) == 0 {
		fmt.Fprintf(os.Stderr, "failed to find/read public SSH key files\n")
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "%s\n", PublicSSHKeys)

	// reading wireguard public key
	cmd := exec.Command("/bin/sh", "-c", "/bin/wg pubkey < wireguard/private")
	if out, err := cmd.Output(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to start wireguard for public-key derivation: %+v\n", err)
		os.Exit(1)
	} else {
		PublicWireguardKey = string(out)
	}

	err = exec.Command("/bin/sh", "-c", "ip link del dev avaron||:").Run()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error removing prior interface: %+v\n", err)
		os.Exit(1)
	}

	ctx := context.Background()
	Serve(ctx)
}
