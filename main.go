package main

import (
	"avaron/llama"
	network "avaron/net"
	"avaron/whois"
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	systemd "github.com/coreos/go-systemd/v22/dbus"
	"io"
	"io/fs"
	"log"
	"math"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/user"
	filepath "path"
	"strconv"
	"strings"
	"syscall"
	"time"
)

var (
	IPv6PeerToPeerMask = [16]byte{
		0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFC,
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

	log.Printf("decoding buf: '%s'\n", buf[:])
	_, err := base64.StdEncoding.Decode(k[:], bytes.TrimSpace(buf[:]))
	return int64(len(buf)), err
}

func (k Key) MarshalText() ([]byte, error) {
	buf := make([]byte, 44)
	base64.StdEncoding.Encode(buf[:], k[:])
	return buf, nil
}

func (k Key) AsPath() string {
	return strings.Replace(k.String(), "/", "-", -1)
}

func truncate(f float64, precision int) float64 {
	shift := math.Pow(10, float64(precision))
	return math.Trunc(f*shift) / shift
}

type Point interface {
	Latitude() float64
	Longitude() float64
}

func ToCoordinates(p Point) [2]float64 {
	return [2]float64{p.Longitude(), p.Latitude()}
}

const HEALTH_PROMPT = `
You are a Linux Network Engineer. Your job is to diagnose network configurations.
If the information you're given is indicative of a HEALTHY network configuration, say HEALTHY.
If it's UNHEALTHY, say UNHEALTHY, followed by the command you'd like to run for further diagnostics.

Here is an example:
HEALTHY
Everything looks good

Here is another example:
UNHEALTHY
$ ip -br link show
Let's run "ip -br link show" to see more about the links

And another:
HEALTHY
docker0 is down. who gives a shit

And another:
UNHEALTHY
$ ping 8.8.8.8
there's not enough information here. let's try running "ping 8.8.8.8" to confirm that we're able to access the internet.

`

func HealthCheck(ctx context.Context) (messages []Message, err error) {
	r, w := io.Pipe()
	go func() {
		err = network.ListBrief(ctx, w)
	}()

	buf, _ := io.ReadAll(r)
	if err != nil {
		return
	}

	messages = []Message{
		{
			Content: fmt.Sprintf("%s\nthe following is the output of ip -br addr show: %s\n", HEALTH_PROMPT, string(buf)),
			Role:    "user",
		},
	}

	for {
		builder := &strings.Builder{}
		for _, m := range messages {
			switch m.Role {
			case "user":
				fmt.Fprintf(builder, "[INST]%s[/INST]", m.Content)
			default:
				fmt.Fprintf(builder, "%s", m.Content)
			}
		}
		prompt := builder.String()
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

		log.Println("got response - starting scanning:", res)

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

		buf, _ = io.ReadAll(io.TeeReader(r, os.Stderr))
		prompt = string(buf)
		log.Println("got full response:", prompt)
		messages = append(messages, Message{
			Role:    "assistant",
			Content: prompt,
		})

		var i int
		if i = strings.Index(prompt, "\n$ "); i == -1 || strings.Index(prompt, "UNHEALTHY") == -1 {
			break
		}

		shell := prompt[i+3:]
		if i = strings.Index(shell, "\n"); i != -1 {
			shell = shell[:i]
		}

		shell = strings.TrimSpace(shell)
		if shell == "" {
			break
		}
		log.Println("running suggested command:", shell)

		out, err := exec.CommandContext(ctx, "/bin/sh", "-c", shell).CombinedOutput()
		if err != nil {
			log.Printf("failed to run ai suggestion: %+v\n", err)
			break
		}

		log.Printf("output from suggested command:\n%s", string(out))

		messages = append(messages, Message{
			Role:    "user",
			Content: string(out),
		})
	}
	return
}

var HealthCheckerRequests = make(chan io.WriteCloser)

func HealthChecker(ctx context.Context) {
	ticker := time.NewTicker(time.Minute * 10)
	messages := make(map[time.Time][]Message)
	var (
		t           time.Time
		channel, ch chan []Message
		ticks       <-chan time.Time
	)

	channel = make(chan []Message)
	ticks = ticker.C

	for {
		select {
		case w := <-HealthCheckerRequests:
			enc := json.NewEncoder(w)
			err := enc.Encode(messages)
			if err != nil {
				log.Println("error encoding:", err)
			}
			w.Close()
		case <-ticks:
			ticks, ch = nil, channel
			t = time.Now()
			log.Println("HealthChecker tick")
			go func() {
				messages, err := HealthCheck(ctx)
				if err != nil {
					log.Println("HealthCheck error:", err)
				}
				select {
				case ch <- messages:
				case <-ctx.Done():
				}
			}()
		case m := <-ch:
			ticks, ch = ticker.C, nil
			if m != nil {
				log.Printf("received %d messages: %+v\n", len(m), messages)
				messages[t] = m
			}
		case <-ctx.Done():
			return
		}
	}
}

func GenerateLinkLocal(k1, k2 *Key) (n1, n2 net.IPNet) {
	if len(k1) != len(k2) {
		panic("keys should be same length")
	}
	if len(k1) < net.IPv6len {
		panic("key should be longer than IPv6 address")
	}
	log.Printf("XORING\n%s\n%s\n", k1.String(), k2.String())

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
		n1.IP[i] = (n1.IP[i] & 0xfc) | 0x01
		n2.IP[i] = (n2.IP[i] & 0xfc) | 0x02
	} else if cmp > 0 {
		n1.IP[i] = (n1.IP[i] & 0xfc) | 0x02
		n2.IP[i] = (n2.IP[i] & 0xfc) | 0x01
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

// Node represents the branch office information
type Node struct {
	Name       string                        `json:"name"`
	Location   *whois.Info                   `json:"location"`
	Interfaces map[string]*network.Interface `json:"interfaces"`
	TCPMetrics []network.TCPMetric           `json:"metrics"`
	Routes     []network.Route               `json:"metrics"`
}

func ListServices(ctx context.Context) (m map[string]systemd.UnitStatus, err error) {
	var conn *systemd.Conn
	conn, err = systemd.NewSystemConnectionContext(ctx)
	if err != nil {
		fmt.Printf("Failed to connect to systemd: %v\n", err)
		return
	}
	defer conn.Close()

	var files []systemd.UnitFile
	files, err = conn.ListUnitFilesByPatternsContext(ctx, nil, nil)
	if err != nil {
		log.Println("failed to list unit-files:", err)
		return
	}

	paths := make([]string, 0, len(files))
	m = make(map[string]systemd.UnitStatus)
	for i := range files {
		path := filepath.Base(files[i].Path)
		if !strings.HasSuffix(path, ".service") {
			continue
		}
		if strings.Index(path, "@.") >= 0 {
			continue
		}
		if strings.Index(path, "systemd") >= 0 {
			continue
		}
		if files[i].Type == "transient" {
			continue
		}
		paths = append(paths, path)
	}

	units, err := conn.ListUnitsByNamesContext(ctx, paths)
	if err != nil {
		log.Println("failed to list units:", err)
		return
	}

	m = make(map[string]systemd.UnitStatus)

	for _, unit := range units {
		m[unit.Name] = unit
	}

	return
}

func GetNode(ctx context.Context) (node Node, err error) {
	node.Name, err = os.Hostname()
	if err != nil {
		err = fmt.Errorf("failed to query OS hostname: %+v", err)
		return
	}

	node.Interfaces, err = network.List(ctx)
	if err != nil {
		err = fmt.Errorf("failed to query network links: %+v", err)
		return
	}

	node.Routes, err = network.Routes(ctx)
	if err != nil {
		err = fmt.Errorf("failed to query routes: %+v", err)
		return
	}

	node.TCPMetrics, err = network.Metrics(ctx)
	if err != nil {
		err = fmt.Errorf("failed to query network metrics: %+v", err)
	}

	return
}

var (
	Home               fs.FS
	PublicSSHKeys      string
	PublicWireguardKey Key
	WhoisInfo          whois.Info
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
		peer := fmt.Sprintf("http://%s/api/keys/wireguard", os.Args[2])
		r1, err := http.Get(peer)
		if err != nil {
			return err
		}
		defer r1.Body.Close()

		if r1.StatusCode != http.StatusOK {
			return fmt.Errorf("%s responded with %s", peer, r1.Status)
		}

		buf, err := io.ReadAll(r1.Body)
		if err != nil {
			return fmt.Errorf("reading response body: %+v", err)
		}

		var key Key
		_, err = key.UnmarshalText(bytes.TrimSpace(buf))
		if err != nil {
			return fmt.Errorf("failed to parse response as Wireguard Key: %+v", err)
		}

		peer = fmt.Sprintf("http://%s/api/keys/ssh", os.Args[2])
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

		dir := filepath.Join("peers", key.AsPath())
		err = os.Mkdir(dir, 0755)
		if err != nil {
			return fmt.Errorf("reading response body: %+v", err)
		}

		// TODO: rm -rf on failure
		url, _ := url.Parse(peer)
		host := url.Host
		if i := strings.Index(host, ":"); i > 0 {
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

		log.Printf("got public keys - wg: %s, ssh: %s\n", key.String(), string(ssh))

		peers, err := GetPeerInfo()
		if err != nil {
			return fmt.Errorf("failed getting peers: %+v\n", err)
		}

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		r, w := io.Pipe()
		go func() {
			WritePeerConfiguration(w, &PublicWireguardKey, peers)
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

type PeerInfo interface {
	IP() string
}

type PeerFSEntry struct {
	address string
}

func (p *PeerFSEntry) IP() string {
	return p.address
}

func (k *Key) Sync(ctx context.Context, ch chan pair) error {
	addr := k.GlobalAddress()
	host := fmt.Sprintf("%s:8080", addr.IP.String())
	fmt.Printf("fetching branch updates from %+v\n", host)
	req := &http.Request{
		URL: &url.URL{
			Scheme:      "http",
			Host:        host,
			Path:        "/api/sdwan",
			Fragment:    "",
			RawFragment: "",
		},
		Proto:      "HTTP/1.1",
		ProtoMajor: 1,
		ProtoMinor: 1,
		Header: map[string][]string{
			"User-Agent": {"Avaron-Core"},
		},
	}

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	dec := json.NewDecoder(res.Body)

	if t, _ := dec.Token(); t != json.Delim('[') {
		return fmt.Errorf("expected '[' as starting delimeter")
	}

	var node Node

	for dec.More() {
		key, err := dec.Token()
		if err != nil {
			return err
		}

		err = dec.Decode(&node)
		if err != nil {
			return err
		}

		select {
		case ch <- struct {
			string
			Node
		}{key.(string), node}:
		case <-ctx.Done():
			return nil
		}
	}

	if t, _ := dec.Token(); t != json.Delim(']') {
		return fmt.Errorf("expected ']' as starting delimeter")
	}

	return nil
}

func GetPeerInfo() (map[Key]PeerInfo, error) {
	entries, err := os.ReadDir("peers")

	peers := make(map[Key]PeerInfo, len(entries))
	if err != nil {
		log.Println("failed to read peers directory:", err)
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
		log.Printf("parsed key: %s\n", k.String())
		dir := filepath.Join("peers", entry.Name())
		address, err := os.ReadFile(filepath.Join(dir, "address"))
		if err != nil {
			return peers, fmt.Errorf("failed to read address for peer '%s': %+v\n", entry.Name(), err)
		}
		peers[*k] = &PeerFSEntry{
			address: string(bytes.TrimSpace(address)),
		}
		log.Printf("read peer: %s, %s\n", k.String(), string(address))
	}

	return peers, nil
}

func Shell(ctx context.Context, r io.Reader) error {
	cmd := exec.CommandContext(ctx, "/usr/bin/sh", "-e")
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

func WritePeerConfiguration(w io.Writer, us *Key, peers map[Key]PeerInfo) (n int, e error) {
	for key, peer := range peers {
		ours, theirs := GenerateLinkLocal(us, &key)
		remote := key.GlobalAddress()
		n, e = fmt.Fprintf(w, "sudo wg set avaron peer %s endpoint %s:51820 allowed-ips ::/0\n", key, peer.IP())
		if e != nil {
			return
		}
		n, e = fmt.Fprintf(w, "sudo /usr/sbin/ip address replace dev avaron %s\n", ours.String())
		if e != nil {
			return
		}
		n, e = fmt.Fprintf(w, "sudo /usr/sbin/ip route replace %s via %s dev avaron\n", remote.String(), theirs.IP.String())
		if e != nil {
			return
		}
	}
	return
}

type pair struct {
	string
	Node
}

var (
	UpdatePeer   = make(chan pair)
	RequestPeers = make(chan io.WriteCloser)
)

func main() {
	log.SetFlags(log.Lshortfile)
	if len(os.Args) < 1 {
		log.Fatalf("unnamed binary\n")
	}

	base := filepath.Base(os.Args[0])
	user, err := user.Lookup(base)
	if err != nil {
		log.Fatalf("failed to find user '%s' - %+v\n", base, err)
	}

	uid, err := strconv.Atoi(user.Uid)
	if err != nil {
		log.Fatalf("failed to parse %s's UID '%s' - %+v\n", base, user.Uid, err)
	}

	err = syscall.Setuid(uid)
	if err != nil {
		log.Fatalf("failed switching to UID %d\n", uid)
	}

	puid := os.Getuid()
	if puid != 0 && puid != uid {
		log.Fatalf("%s was invoked by UID %d, however, the %s user has UID %d\n", os.Args[0], puid, filepath.Base(os.Args[0]), uid)
	}

	if err := os.Chdir(user.HomeDir); err != nil {
		log.Println("changing to home directory:", err)
		os.Exit(1)
	}

	createPIDFile := func() {
		buf := fmt.Sprintf("%d\n", os.Getpid())
		err := os.WriteFile("pid", []byte(buf), 0644)
		if err != nil {
			log.Println("failed to create PID file:", err)
			os.Exit(1)
		}
	}

	// reading all SSH public keys
	ssh := os.DirFS(".ssh")

	paths, err := fs.Glob(ssh, "*.pub")
	if err != nil {
		log.Println("failed to find public SSH keys:", err)
		os.Exit(1)
	}
	log.Printf("found %d SSH public keys: %s\n", len(paths), strings.Join(paths, ", "))

	for _, path := range paths {
		pub, err := fs.ReadFile(ssh, path)
		if err != nil {
			log.Printf("error reading %s: %+v\n", path, err)
			continue
		}
		PublicSSHKeys += string(pub)
	}

	if len(PublicSSHKeys) == 0 {
		log.Fatalf("failed to find/read public SSH key files\n")
	}

	log.Printf("%s\n", PublicSSHKeys)

	log.Printf("getting wireguard public key...\n")

	// reading wireguard public key
	cmd := exec.Command("/usr/bin/sh", "-c", "/usr/bin/wg pubkey < wireguard/private")
	if out, err := cmd.Output(); err != nil {
		log.Println("failed to start wireguard for public-key derivation:", err)
		os.Exit(1)
	} else if _, err = PublicWireguardKey.UnmarshalText(out); err != nil {
		log.Println("failed to parse wireguard key:", err)
		os.Exit(1)
	}

	buf, err := os.ReadFile("pid")
	if err != nil && os.IsNotExist(err) {
		if len(os.Args) > 1 {
			log.Fatalf("attempted to invoking controller without existing process\n")
		}
		createPIDFile()
	} else if err != nil {
		log.Println("failed to read PID file:", err)
		os.Exit(1)
	} else if pid, err := strconv.Atoi(string(bytes.TrimSpace(buf))); err != nil {
		log.Println("failed to read PID file:", err)
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
				log.Println(err)
				os.Exit(1)
			}
			os.Exit(0)
		} else {
			createPIDFile()
		}
	}

	ctx, _ := context.WithCancel(context.Background())

	go ServeHTTP(ctx)
	go HealthChecker(ctx)

	links, err := network.List(ctx)
	if err != nil {
		log.Println("failed to probe network links:", err)
		os.Exit(1)
	}

	peers, err := GetPeerInfo()
	if err != nil {
		log.Println("failed getting peers:", err)
		os.Exit(1)
	}

	{
		ctx, cancel := context.WithCancel(context.Background())
		r, w := io.Pipe()
		go func() {
			if _, ok := links["avaron"]; !ok {
				fmt.Fprintf(w, "sudo /usr/sbin/ip link add dev avaron type wireguard\n")
			}

			local := PublicWireguardKey.GlobalAddress()
			fmt.Fprintf(w, "sudo /usr/sbin/ip address replace dev avaron %s\n", local.String())
			fmt.Fprintf(w, "sudo /usr/bin/wg set avaron listen-port %d private-key %s\n", 51820, "wireguard/private")
			fmt.Fprintf(w, "sudo /usr/sbin/ip link set up dev avaron\n")
			WritePeerConfiguration(w, &PublicWireguardKey, peers)
			w.Close()
		}()
		err = Shell(ctx, r)
		if err != nil {
			log.Println("failed writing network configuration to shell:", err)
			os.Exit(1)
		}
		cancel()

	}

	log.Printf("iterating over %d peers\n", len(peers))

	for key := range peers {
		go func(key Key) {
			ticker := time.NewTicker(time.Second * 5)
			for {
				select {
				case <-ticker.C:
				case <-ctx.Done():
					return
				}
				err := key.Sync(ctx, UpdatePeer)
				if err != nil {
					log.Println("error fething updates:", err)
				}
			}
		}(key)
	}

	WhoisInfo, err = whois.Get()
	if err != nil {
		log.Println("failed to get coordinates:", err)
	}

	nodes := make(map[Key]Node)
	for {
		select {
		case pair := <-UpdatePeer:
			fmt.Printf("updating nodes\n")
			var k Key
			_, err := k.UnmarshalText([]byte(pair.string))
			if err != nil {
				log.Printf("failed to parse peer ID: %s\n", pair.string)
				continue
			}

			if bytes.Equal(k[:], PublicWireguardKey[:]) {
				continue
			}

			_, ok := nodes[k]
			if !ok {
				log.Printf("unfound peer: %s\n", k.String())
				continue
			}
			nodes[k] = pair.Node
		case w := <-RequestPeers:
			fmt.Printf("requesting nodes\n")

			var buf []byte
			var err error

			if nodes[PublicWireguardKey], err = GetNode(ctx); err != nil {
				log.Printf("failed to get local branch: %+v", err)
			} else if buf, err = json.Marshal(nodes); err != nil {
				log.Printf("failed to marshal nodes: %+v", err)
			} else if _, err = w.Write(buf); err != nil {
				log.Println("error writing nodes:", err)
			}

			w.Close()
		case <-ctx.Done():
			return
		}
	}
}
