package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	nl "github.com/vishvananda/netlink"
	"io"
	"io/fs"
	"net"
	"net/http"
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

// Location represents the geographical location of the branch
type Location struct {
	Latitude  float64 `json:"lat"`
	Longitude float64 `json:"lng"`
	Address   string  `json:"address"`
}

// Bandwidth represents the bandwidth details for a connection
type Bandwidth struct {
	Download int `json:"download"`
	Upload   int `json:"upload"`
}

// Connection represents a network connection
type Connection struct {
	ID        string    `json:"id"`
	Type      string    `json:"type"`
	Status    string    `json:"status"`
	Uptime    int64     `json:"uptime"`
	Bandwidth Bandwidth `json:"bandwidth"`
}

// Metrics represents various metrics related to the branch
type Metrics struct {
	Latency           float64 `json:"latency"`
	PacketLoss        float64 `json:"packetLoss"`
	Jitter            float64 `json:"jitter"`
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
		res.Body = io.NopCloser(bytes.NewReader(PublicSSHKeys[:]))
	case "/keys/wireguard":
		if req.Method != "GET" {
			res.StatusCode = http.StatusMethodNotAllowed
			break
		}
		res.Body = io.NopCloser(bytes.NewReader(PublicWireguardKey[:]))
	case "/pair":
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

		// decode
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

		fmt.Fprintf(os.Stderr, "got buffer! %+v\n", public)
	}

failure:
	res.Status = http.StatusText(res.StatusCode)

	err = res.Write(conn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error writing request: %+v", err)
		return
	}
}

func sortRoutes(routes []nl.Route) {
	sort.Slice(routes, func(i, j int) bool {
		if !bytes.Equal(routes[i].Dst.Mask, routes[j].Dst.Mask) {
			return bytes.Compare(routes[i].Dst.Mask, routes[j].Dst.Mask) < 0
		}

		if routes[i].Priority != routes[j].Priority {
			return routes[i].Priority < routes[j].Priority
		}
		return false
	})
}

var (
	Home               fs.FS
	PublicSSHKeys      []byte
	PublicWireguardKey []byte
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
	case "pair":
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

func main() {
	if len(os.Args) < 1 {
		fmt.Fprintf(os.Stderr, "unnamed binary")
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
			fmt.Fprintf(os.Stderr, "%s process already exists with pid %d - invoking controller\n", base, pid)
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
		PublicSSHKeys = append(PublicSSHKeys, pub...)
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
		PublicWireguardKey = out
	}

	links, err := nl.LinkList()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error getting interfaces: %+v\n", err)
		os.Exit(1)
	}

	for _, link := range links {
		attrs := link.Attrs()
		fmt.Fprintf(os.Stderr, "name: %s, mac: %s, flags: %s\n", attrs.Name, attrs.HardwareAddr, attrs.Flags)
	}

	routes, err := nl.RouteList(nil, nl.FAMILY_V4)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error getting routes: %+v\n", err)
		os.Exit(1)
	}

	sortRoutes(routes)
	for _, route := range routes {
		fmt.Fprintf(os.Stderr, "route: %s\n", route.Dst.String())
	}

	listener, err := net.Listen("tcp", ":8080")
	if err != nil {
		fmt.Fprintf(os.Stderr, "error starting listener: %+v\n", err)
		return
	}
	defer listener.Close()

	fmt.Fprintf(os.Stderr, "listening on %s\n", listener.Addr().String())

	// load balancer - connection times out quicker the more connections there are
	const (
		total   = 256
		timeout = 10 * time.Second
	)
	tokens := make(chan struct{})
	duration := make(chan time.Duration)

	ctx := context.Background()

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
				case duration <- d:
				case <-ctx.Done():
					return
				}
			} else {
				select {
				case <-tokens:
					n++
				case duration <- d:
				case <-ctx.Done():
					return
				}
			}

		}
	}()

	for {
		conn, err := listener.Accept()
		if err != nil {
			fmt.Fprintf(os.Stderr, "error accepting connection: %+v\n", err)
			continue
		}

		dur, ok := <-duration
		if !ok {
			break
		}
		fmt.Fprintf(os.Stderr, "dur: %20s\n", dur)

		t := time.Now().Add(dur)
		conn.SetDeadline(t)
		deadline, _ := context.WithDeadline(ctx, t)
		go func() {
			select {
			case <-tokens:
				// borrow token
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
