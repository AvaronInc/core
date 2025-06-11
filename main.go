package main

import (
	"avaron/llama"
	network "avaron/net"
	"avaron/vertex"
	"avaron/whois"
	wg "avaron/wireguard"
	"bytes"
	"context"
	_ "embed"
	"avaron/health"
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
	"text/template"
	"time"
)

var (
	IPv6PeerToPeerMask = [16]byte{
		0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFC,
	}
)

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

func GenerateLinkLocal(k1, k2 *vertex.Key) (n1, n2 net.IPNet) {
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

type Node struct {
	Name       string                        `json:"name"`
	Location   *whois.Info                   `json:"location"`
	Interfaces map[string]*network.Interface `json:"interfaces"`
	Tunnels    map[vertex.Key]*wg.Interface  `json:"tunnels"`
	TCPMetrics []network.TCPMetric           `json:"metrics"`
	Routes     map[string]*network.Route     `json:"routes"`
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
	node.Location = &WhoisInfo

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

	node.Tunnels, err = wg.Interfaces(ctx)
	if err != nil {
		err = fmt.Errorf("failed to query network metrics: %+v", err)
	}

	return
}

var (
	Home               fs.FS
	PublicSSHKeys      string
	PublicWireguardKey vertex.Key
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

		var key vertex.Key
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

		dir := filepath.Join("peers", key.Path())
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

func Sync(ctx context.Context, k *vertex.Key, ch chan pair) error {
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

func GetPeerInfo() (map[vertex.Key]PeerInfo, error) {
	entries, err := os.ReadDir("peers")

	peers := make(map[vertex.Key]PeerInfo, len(entries))
	if err != nil {
		log.Println("failed to read peers directory:", err)
		// this is fine
		return peers, nil
	}

	for _, entry := range entries {
		k := new(vertex.Key)
		text := strings.Replace(entry.Name(), "-", "/", -1)
		_, err := k.UnmarshalText([]byte(text))
		if err != nil {
			return peers, fmt.Errorf("failed to parse key '%s': %+v\n", entry.Name(), err)
		}
		log.Printf("parsed key: %s\n", k.String())
		dir := filepath.Join("peers", entry.Name())
		address, err := os.ReadFile(filepath.Join(dir, "address"))
		if err == nil {
			peers[*k] = &PeerFSEntry{
				address: string(bytes.TrimSpace(address)),
			}
		} else if os.IsNotExist(err) {
			peers[*k] = &PeerFSEntry{}
		} else if err != nil {
			return peers, fmt.Errorf("failed to read address for peer '%s': %+v\n", entry.Name(), err)
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

func WritePeerConfiguration(w io.Writer, us *vertex.Key, peers map[vertex.Key]PeerInfo) (n int, e error) {
	n, e = fmt.Fprintf(w, "sudo /usr/sbin/ip route replace fc00:a7a0::/32 dev avaron src %s\n", PublicWireguardKey.GlobalAddress().IP.String())
	if e != nil {
		return
	}

	for key, peer := range peers {
		ours, theirs := GenerateLinkLocal(us, &key)
		remote := key.GlobalAddress()
		if ip := peer.IP(); ip == "" {
			n, e = fmt.Fprintf(w, "sudo wg set avaron peer %s allowed-ips fc00:a7a0::/32,%s/128\n", key, theirs.IP.String())
		} else {
			n, e = fmt.Fprintf(w, "sudo wg set avaron peer %s endpoint %s:51820 allowed-ips fc00:a7a0::/32\n", key, ip)
		}
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
	UpdateNode   = make(chan pair)
	RequestNodes = make(chan io.WriteCloser)
)

var (
	//go:embed named/conf.template
	NamedConfiguration string

	//go:embed named/zone.template
	NamedZone string
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

	llama.Init()

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

	var file *os.File
	file, err = os.Open("wireguard/private")
	if err != nil {
		log.Println("failed to open wireguard/private:", err)
		os.Exit(1)
	}

	// reading wireguard public key
	if PublicWireguardKey, err = wg.PublicKey(file); err != nil {
		log.Println("failed to dervice public key:", err)
		os.Exit(1)
	}

	log.Println("derived public key:", PublicWireguardKey)

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
	go health.Loop(ctx)

	{
		named := exec.CommandContext(ctx, "/bin/sudo", "-S", "/usr/local/bin/named", "-f", "-g", "-c", "/tmp/conf")
		//named.Stderr = os.Stderr
		//named.Stdout = os.Stderr

		dir := os.Getenv("NAMED_DIR")
		if dir == "" {
			dir = "/tmp"
		}

		os.Remove("/tmp/zone")
		f, err := os.OpenFile("/tmp/zone", os.O_CREATE|os.O_WRONLY, 0666)
		if err != nil {
			log.Println("error opening /tmp/zone", err)
			os.Exit(1)

		}

		t, err := template.New("").Parse(NamedZone)
		if err != nil {
			log.Println("failed parsing it", NamedZone)
		}

		err = t.Execute(f, struct {
			IPv6 string
		}{
			PublicWireguardKey.GlobalAddress().IP.String(),
		})
		if err != nil {
			log.Println("error executing template:", err)
			os.Exit(1)
		}

		f.Close()

		os.Remove("/tmp/conf")
		f, err = os.OpenFile("/tmp/conf", os.O_CREATE|os.O_WRONLY, 0666)
		if err != nil {
			log.Println("error opening /tmp/zone", err)
			os.Exit(1)

		}

		t, err = template.New("").Parse(NamedConfiguration)
		if err != nil {
			log.Println("failed parsing it", NamedConfiguration)
		}

		t.Execute(f, struct {
			Directory     string
			ProcessIDFile string
			Reverse       string
			Zone          string
		}{
			"/tmp",
			"/tmp/named-pid",
			"",
			"/tmp/zone",
		})
		if err != nil {
			log.Println("error executing template:", err)
			os.Exit(1)
		}

		f.Close()

		err = named.Start()
		if err != nil {
			log.Println("failed starting named:", err)
			os.Exit(1)
		}

		go func() {
			err := named.Wait()
			if err != nil {
				log.Panicln("named exited:", err)
			}
		}()
	}

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
		go func(key vertex.Key) {
			ticker := time.NewTicker(time.Second * 5)
			for {
				select {
				case <-ticker.C:
				case <-ctx.Done():
					return
				}
				err := Sync(ctx, &key, UpdateNode)
				if err != nil {
					log.Println("error fetching updates:", err)
				}
			}
		}(key)
	}

	WhoisInfo, err = whois.Get()
	if err != nil {
		log.Println("failed to get coordinates:", err)
	} else {
		log.Println("got coordinates", WhoisInfo)
	}

	nodes := make(map[vertex.Key]Node)
	for {
		select {
		case pair := <-UpdateNode:
			fmt.Printf("updating nodes\n")
			var k vertex.Key
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
		case w := <-RequestNodes:
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
