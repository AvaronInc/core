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

var (
	IPv6PeerToPeerMask = [16]byte{
		0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF ^ 0x03,
	}
)

type Key [32]byte

func (k Key) String() string {
	return base64.StdEncoding.EncodeToString(k[:])
}

func (k *Key) UnmarshalText(buf []byte) (int64, error) {
	if len(buf) < 44 {
		return 0, io.ErrShortBuffer
	}

	fmt.Fprintf(os.Stderr, "decoding buf: %s\n", buf[:44])
	_, err := base64.StdEncoding.Decode(k[:], buf[:44])
	if err != nil {
		return 0, err
	}
	return 44, err
}

func (k Key) MarshalText() ([]byte, error) {
	buf := make([]byte, 44)
	base64.StdEncoding.Encode(buf[:], k[:])
	return buf, nil
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

func truncate(f float64, precision int) float64 {
	shift := math.Pow(10, float64(precision))
	return math.Trunc(f*shift) / shift
}

func GenerateLinkLocal(k1, k2 Key) (n1, n2 net.IPNet) {
	if len(k1) != len(k2) {
		panic("keys should be same length")
	}
	if len(k1) < net.IPv6len {
		panic("key should be longer than IPv6 address")
	}

	var (
		prefix = []byte{0xfe, 0x80}
		i, cmp int
	)

	n1.IP = make([]byte, net.IPv6len)
	n1.Mask = make([]byte, net.IPv6len)
	n2.IP = make([]byte, net.IPv6len)
	n2.Mask = make([]byte, net.IPv6len)
	copy(n1.IP, prefix)
	copy(n2.IP, prefix)
	copy(n1.Mask, IPv6PeerToPeerMask[:])
	copy(n2.Mask, IPv6PeerToPeerMask[:])

	for i = 0; i < net.IPv6len-len(prefix); i++ {
		if cmp != 0 {
			// ok
		} else if k1[i] < k2[i] {
			cmp = -1
		} else if k1[i] > k2[i] {
			cmp = 1
		}
		n1.IP[i+len(prefix)] = k1[i] ^ k2[i]
		n2.IP[i+len(prefix)] = k1[i] ^ k2[i]
	}

	i = net.IPv6len - 1

	if cmp < 0 {
		n1.IP[i] = (k1[i] & 0xf0) | 0x01
		n2.IP[i] = (k2[i] & 0xf0) | 0x02
	} else if cmp > 0 {
		n1.IP[i] = (k1[i] & 0xf0) | 0x02
		n2.IP[i] = (k2[i] & 0xf0) | 0x01
	} else {
		panic("cowardly refusing to generate link-local addresses for the same host")
	}

	return
}

func (k Key) GlobalAddress() (n net.IPNet) {
	n.IP = make([]byte, net.IPv6len)
	n.Mask = make([]byte, net.IPv6len)

	if len(k) < net.IPv6len {
		panic("key should be longer than IPv6 address")
	}

	n.Mask[0] = 0xff
	n.Mask[1] = 0xff

	var (
		prefix = []byte{0xfc, 0x00, 0xa7, 0xa0}
		mask   = []byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff}
	)

	copy(n.IP, prefix)
	copy(n.Mask, mask)

	for i := 0; i < net.IPv6len-len(prefix); i++ {
		n.IP[i+len(prefix)] = k[i]
	}

	return
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
		res.Body = io.NopCloser(bytes.NewReader(PublicWireguardKey[:]))
	case "/link":
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
		r := base64.NewDecoder(base64.StdEncoding, io.LimitReader(req.Body, 44))

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
			ID:            PublicWireguardKey.String(),
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
	PublicWireguardKey Key
)

func controller() error {
	if len(os.Args) <= 1 {
		return fmt.Errorf("controller invoked with no arguments")
	}

	switch os.Args[1] {
	case "peer":
		if len(os.Args) <= 2 {
			return fmt.Errorf("not enough arguments")
		}
		peer := fmt.Sprintf("http://%s/keys/wireguard", os.Args[2])
		r1, err := http.Get(peer)
		if err != nil {
			return err
		}
		defer r1.Body.Close()

		if r1.StatusCode != http.StatusOK {
			return fmt.Errorf("%s responded with %s", peer, r1.Status)
		}

		wireguard, err := io.ReadAll(r1.Body)
		if err != nil {
			return fmt.Errorf("reading response body: %+v", err)
		}

		peer = fmt.Sprintf("http://%s/keys/ssh", os.Args[2])
		r2, err := http.Get(peer)
		if err != nil {
			return err
		}
		defer r2.Body.Close()

		if r2.StatusCode != http.StatusOK {
			return fmt.Errorf("%s responded with %s", peer, r2.Status)
		}

		ssh, err := io.ReadAll(r2.Body)
		if err != nil {
			return fmt.Errorf("reading response body: %+v", err)
		}

		wireguard = bytes.TrimSpace(wireguard)
		dir := filepath.Join("peers", strings.Replace(string(wireguard), "/", "-", -1))
		err = os.Mkdir(dir, 0755)
		if err != nil {
			return fmt.Errorf("reading response body: %+v", err)
		}

		// TODO: rm -rf on failure
		url, _ := url.Parse(peer)
		host := url.Host
		if i := strings.Index(host, ":"); i != 0 {
			host = host[:i]
		}
		host = fmt.Sprintf("%s\n", host)
		err = os.WriteFile(fmt.Sprintf("%s/address", dir), []byte(host), 0555)
		if err != nil {
			return fmt.Errorf("failed writing host '%s' to file: %+v", url.Host, err)
		}

		err = os.WriteFile(fmt.Sprintf("%s/ssh", dir), ssh, 0555)
		if err != nil {
			return fmt.Errorf("failed writing ssh file: %+v", err)
		}

		fmt.Fprintf(os.Stderr, "got public keys - wg: %s, ssh: %s\n", string(wireguard), string(ssh))

		peers, err := GetPeers()
		if err != nil {
			return fmt.Errorf("failed getting peers: %+v\n", err)
		}

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		r, w := io.Pipe()
		ch := make(chan Key)

		go func() {
			defer close(ch)
			for key := range peers {
				select {
				case ch <- key:
				case <-ctx.Done():
					return
				}
			}
		}()

		go func() {
			WritePeerConfiguration(w, PublicWireguardKey, ch)
			w.Close()
		}()
		if err = Shell(ctx, r); err != nil {
			return fmt.Errorf("failed writing network peer configuration to shell: %+v\n", err)
		}
	default:
		return fmt.Errorf("unknown option: %s", os.Args[1])
	}

	return nil
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

type Peer interface {
	URL() *url.URL
	Branch() *Branch
}

type peerFS struct {
	address string
	branch  Branch
}

func (p *peerFS) URL() *url.URL {
	var host string
	host = fmt.Sprintf("%s", p.address)
	return &url.URL{
		Scheme:      "http",
		Host:        host,
		Path:        "/sdwan",
		Fragment:    "",
		RawFragment: "",
	}
}

func (p *peerFS) Branch() *Branch {
	return &p.branch
}

func GetPeers() (map[Key]Peer, error) {
	entries, err := os.ReadDir("peers")

	peers := make(map[Key]Peer, len(entries))
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to read peers directory: %+v\n", err)
		// this is fine
		return peers, nil
	}

	for _, entry := range entries {
		k := new(Key)
		text := strings.Replace(entry.Name(), "-", "/", -1)
		_, err := k.UnmarshalText([]byte(text))
		if err != nil {
			return peers, fmt.Errorf("failed to parse key '%s': %+v\n", entry.Name(), err)
		}
		dir := filepath.Join("peers", entry.Name())
		address, err := os.ReadFile(filepath.Join(dir, "address"))
		if err != nil {
			return peers, fmt.Errorf("failed to read address for peer '%s': %+v\n", entry.Name(), err)
		}
		peers[*k] = &peerFS{
			address: string(address),
		}
		fmt.Fprintf(os.Stderr, "read peer: %s, %s\n", k.String(), string(address))
	}

	return peers, nil
}

func Shell(ctx context.Context, r io.Reader) error {
	cmd := exec.CommandContext(ctx, "/bin/sh", "-e")
	w, err := cmd.StdinPipe()
	if err != nil {
		//os.Fprintf(os.Stderr, "failed to create shell pipe: %+v\n", err)
		//os.Exit(1)
		return err
	}
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	err = cmd.Start()
	if err != nil {
		//os.Fprintf(os.Stderr, "failed to start shell: %+v\n", err)
		//os.Exit(1)
		return err
	}
	_, e1 := io.Copy(w, io.TeeReader(r, os.Stderr))
	w.Close()
	e2 := cmd.Wait()
	if e2 != nil {
		return e2
	}
	return e1
}

func WritePeerConfiguration(w io.Writer, key Key, peers chan Key) (n int, e error) {
	for key := range peers {
		ours, theirs := GenerateLinkLocal(PublicWireguardKey, key)
		remote := key.GlobalAddress()
		n, e = fmt.Fprintf(w, "sudo ip address replace dev avaron %s\n", ours.String())
		if e != nil {
			return
		}
		n, e = fmt.Fprintf(w, "sudo ip route replace %s via %s dev avaron\n", remote.String(), theirs.IP.String())
		if e != nil {
			return
		}
	}
	return
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

	fmt.Fprintf(os.Stderr, "getting wireguard public key...\n")

	// reading wireguard public key
	cmd := exec.Command("/bin/sh", "-c", "/bin/wg pubkey < wireguard/private")
	if out, err := cmd.Output(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to start wireguard for public-key derivation: %+v\n", err)
		os.Exit(1)
	} else if _, err = PublicWireguardKey.UnmarshalText(out); err != nil {
		fmt.Fprintf(os.Stderr, "failed to parse wireguard key: %+v\n", err)
		os.Exit(1)
	}

	ctx := context.Background()
	go Serve(ctx)

	type Value struct{}

	updates := make(chan Branch)
	client := http.DefaultClient

	links, err := network.List(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to probe network links: %+v\n", err)
		os.Exit(1)
	}

	peers, err := GetPeers()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed getting peers: %+v\n", err)
		os.Exit(1)
	}

	{
		ctx, cancel := context.WithCancel(context.Background())
		r, w := io.Pipe()
		ch := make(chan Key)
		go func() {
			defer close(ch)
			for key := range peers {
				select {
				case ch <- key:
				case <-ctx.Done():
					return
				}
			}
		}()
		go func() {
			if _, ok := links["avaron"]; !ok {
				fmt.Fprintf(w, "sudo ip link add dev avaron type wireguard\n")
			}

			local := PublicWireguardKey.GlobalAddress()
			fmt.Fprintf(w, "sudo ip address replace dev avaron %s\n", local.String())
			fmt.Fprintf(w, "sudo wg set avaron listen-port %d private-key %s\n", 51820, "wireguard/private")
			fmt.Fprintf(w, "sudo ip link set up dev avaron\n")
			WritePeerConfiguration(w, PublicWireguardKey, ch)
			w.Close()
		}()
		err = Shell(ctx, r)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed writing network configuration to shell: %+v\n", err)
			os.Exit(1)
		}
		cancel()

	}

	fmt.Fprintf(os.Stderr, "iterating over %d peers\n", len(peers))

	for key, peer := range peers {
		fetch := func() error {
			req := &http.Request{
				URL:        peer.URL(),
				Proto:      "HTTP/1.1",
				ProtoMajor: 1,
				ProtoMinor: 0,
			}

			res, err := client.Do(req)
			if err != nil {
				return err
			}

			defer res.Body.Close()
			dec := json.NewDecoder(res.Body)

			if t, _ := dec.Token(); t != json.Delim('[') {
				return fmt.Errorf("expected '[' as starting delimeter")
			}

			var branch Branch
			for dec.More() {
				err = dec.Decode(&branch)
				if err != nil {
					return err
				}
				select {
				case updates <- branch:
				case <-ctx.Done():
					return nil
				}
			}

			if t, _ := dec.Token(); t != json.Delim(']') {
				return fmt.Errorf("expected ']' as starting delimeter")
			}

			return nil
		}

		go func(key Key) {
			ticker := time.NewTicker(time.Second * 5)
			for {
				select {
				case <-ticker.C:
				case <-ctx.Done():
					return
				}
				err := fetch()
				if err != nil {
					fmt.Fprintf(os.Stderr, "error fething updates: %+v", err)
				}
			}
		}(key)
	}

	for {
		select {
		case branch := <-updates:
			var k Key
			_, err := k.UnmarshalText([]byte(branch.ID))
			if err != nil {
				panic(err)
			}
			*peers[k].Branch() = branch
		case <-ctx.Done():
			return
		}
	}
}
