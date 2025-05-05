package main

import (
	network "avaron/net"
	"avaron/whois"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	systemd "github.com/coreos/go-systemd/v22/dbus"
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

	fmt.Fprintf(os.Stderr, "decoding buf: '%s'\n", buf[:])
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

func GenerateLinkLocal(k1, k2 *Key) (n1, n2 net.IPNet) {
	if len(k1) != len(k2) {
		panic("keys should be same length")
	}
	if len(k1) < net.IPv6len {
		panic("key should be longer than IPv6 address")
	}
	fmt.Fprintf(os.Stderr, "XORING\n%s\n%s\n", k1.String(), k2.String())

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

/* frontend
{
  id: 'vpp-1',
  uuid: 'f8c3de3d-1d47-4f5a-b898-3f0d1c23a1d9',
  name: 'VPP Firewall',
  type: 'vpp',
  description: 'Vector Packet Processing firewall service for SD-WAN',
  status: 'running',
  cpuUsage: randomResourceUsage(),
  memoryUsage: randomResourceUsage(),
  lastRestart: new Date(Date.now() - 3 * 24 * 60 * 60 * 1000).toISOString(), // 3 days ago
  health: 'ok',
  uptime: '3d 2h 14m',
  assignedResources: {
    cpuCores: 2,
    ram: 4096, // MB
    networkInterfaces: ['eth0', 'eth1']
  },
  networkIO: {
    received: 12458, // KB/s
    transmitted: 9845 // KB/s
  },
  dependencies: ['vpp-route-policy', 'interface-manager'],
  logEntries: generateLogEntries(20)
},
*/

type AssignedResources struct {
	CPUCores          int      `json:"cpuCores"`
	RAM               int      `json:"ram"` // in MB
	NetworkInterfaces []string `json:"networkInterfaces"`
}

type NetworkIO struct {
	Received    int `json:"received"`    // in KB/s
	Transmitted int `json:"transmitted"` // in KB/s
}

type LogEntry struct {
	Timestamp time.Time `json:"timestamp"`
	Message   string    `json:"message"`
}

type Service struct {
	ID                string            `json:"id"`
	UUID              string            `json:"uuid"`
	Name              string            `json:"name"`
	Type              string            `json:"type"`
	Description       string            `json:"description"`
	Status            string            `json:"status"`
	CPUUsage          float64           `json:"cpuUsage"`
	MemoryUsage       float64           `json:"memoryUsage"`
	LastRestart       time.Time         `json:"lastRestart"`
	Health            string            `json:"health"`
	Uptime            string            `json:"uptime"`
	AssignedResources AssignedResources `json:"assignedResources"`
	NetworkIO         NetworkIO         `json:"networkIO"`
	Dependencies      []string          `json:"dependencies"`
	LogEntries        []LogEntry        `json:"logEntries"`
}

func ListServices(ctx context.Context, ch chan Service) (err error) {
	defer close(ch)
	var conn *systemd.Conn
	conn, err = systemd.NewSystemConnectionContext(ctx)
	if err != nil {
		fmt.Printf("Failed to connect to systemd: %v\n", err)
		return
	}
	defer conn.Close()

	var units []systemd.UnitStatus
	units, err = conn.ListUnitsContext(ctx)
	if err != nil {
		fmt.Printf("Failed to list units: %v\n", err)
		return
	}

	for _, unit := range units {
		if !strings.HasSuffix(unit.Name, "service") {
			continue
		}
		var property *systemd.Property
		var cpu, memory uint64
		if unit.SubState != "running" {
			cpu, memory = 0, 0
			continue
		} else {
			if property, err = conn.GetServiceProperty(unit.Name, "MemoryCurrent"); err != nil {
				fmt.Fprintf(os.Stderr, "error getting memory for service '%s': %+v\n", unit.Name, err)
			} else {
				memory = property.Value.Value().(uint64)
			}

			if property, err = conn.GetServiceProperty(unit.Name, "CPUUsageNSec"); err != nil {
				fmt.Fprintf(os.Stderr, "error getting memory for service '%s': %+v\n", unit.Name, err)
			} else {
				cpu = property.Value.Value().(uint64)
			}
			fmt.Println(cpu, memory)
		}

		select {
		case ch <- Service{
			ID:          unit.Name,
			UUID:        unit.Name,
			Name:        unit.Name,
			Type:        unit.JobType,
			Description: unit.Description,
			Status:      unit.SubState,
			MemoryUsage: float64(memory),
			CPUUsage:    float64(cpu),
		}:
		case <-ctx.Done():
			return
		}
	}

	return
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

func GetBranch() (Branch, error) {
	hostname, err := os.Hostname()
	if err != nil {
		return Branch{}, fmt.Errorf("failed to query OS hostname: %+v", err)
	}

	links, err := network.List(context.Background())
	if err != nil {
		return Branch{}, fmt.Errorf("failed to query network links: %+v", err)
	}

	routes, err := network.Routes(context.Background())
	if err != nil {
		return Branch{}, fmt.Errorf("failed to query routes: %+v", err)
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
		return Branch{}, fmt.Errorf("failed to query network metrics: %+v", err)
	}

	metrics, ages := Analyze(links, tcpmetrics)
	branch := Branch{
		ID:            PublicWireguardKey.String(),
		Name:          hostname,
		NetworkStatus: "degraded",
		Metrics:       metrics,
		Location:      MyLocation,
	}

	if len(connections) > 0 {
		c := AsConnection(connections[0])
		c.Uptime = int64(ages[connections[0].IfName])
		branch.PrimaryConnection = c
		if addrs := connections[0].AddrInfo; len(addrs) > 0 {
			sort.Sort(network.AddressMask(addrs))
			branch.IPAddress = addrs[0].Local
		}
		branch.NetworkStatus = c.Status
	}

	if len(connections) > 1 {
		c := AsConnection(connections[1])
		c.Uptime = int64(ages[connections[1].IfName])
		branch.FailoverConnection1 = c
	}

	return branch, nil
}

var (
	Home               fs.FS
	PublicSSHKeys      string
	PublicWireguardKey Key
	MyLocation         Location
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

		fmt.Fprintf(os.Stderr, "got public keys - wg: %s, ssh: %s\n", key.String(), string(ssh))

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

func (k *Key) Sync(ctx context.Context, ch chan Branch) error {
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

	var branch Branch
	for dec.More() {
		err = dec.Decode(&branch)
		if err != nil {
			return err
		}
		select {
		case ch <- branch:
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
		fmt.Fprintf(os.Stderr, "parsed key: %s\n", k.String())
		dir := filepath.Join("peers", entry.Name())
		address, err := os.ReadFile(filepath.Join(dir, "address"))
		if err != nil {
			return peers, fmt.Errorf("failed to read address for peer '%s': %+v\n", entry.Name(), err)
		}
		peers[*k] = &PeerFSEntry{
			address: string(bytes.TrimSpace(address)),
		}
		fmt.Fprintf(os.Stderr, "read peer: %s, %s\n", k.String(), string(address))
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

var (
	UpdatePeer   = make(chan Branch)
	RequestPeers = make(chan net.Conn)
)

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
	cmd := exec.Command("/usr/bin/sh", "-c", "/usr/bin/wg pubkey < wireguard/private")
	if out, err := cmd.Output(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to start wireguard for public-key derivation: %+v\n", err)
		os.Exit(1)
	} else if _, err = PublicWireguardKey.UnmarshalText(out); err != nil {
		fmt.Fprintf(os.Stderr, "failed to parse wireguard key: %+v\n", err)
		os.Exit(1)
	}

	info, err := whois.Get()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to get coordinates: %+v\n", err)
	}

	MyLocation = Location{
		Latitude:  info.Latitude(),
		Longitude: info.Longitude(),
		Address:   info.Address(),
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

	ctx := context.Background()
	go ServeHTTP(ctx)

	links, err := network.List(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to probe network links: %+v\n", err)
		os.Exit(1)
	}

	peers, err := GetPeerInfo()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed getting peers: %+v\n", err)
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
			fmt.Fprintf(os.Stderr, "failed writing network configuration to shell: %+v\n", err)
			os.Exit(1)
		}
		cancel()

	}

	fmt.Fprintf(os.Stderr, "iterating over %d peers\n", len(peers))

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
					fmt.Fprintf(os.Stderr, "error fething updates: %+v", err)
				}
			}
		}(key)
	}

	branches := make(map[Key]*Branch)
	for {
		select {
		case branch := <-UpdatePeer:
			fmt.Printf("updating branches\n")
			var k Key
			_, err := k.UnmarshalText([]byte(branch.ID))
			if err != nil {
				fmt.Fprintf(os.Stderr, "failed to parse peer ID: %s\n", branch.ID)
				continue
			}

			if bytes.Equal(k[:], PublicWireguardKey[:]) {
				continue
			}

			bp, ok := branches[k]
			if !ok {
				fmt.Fprintf(os.Stderr, "unfound peer: %s\n", k.String())
				continue
			}
			*bp = branch
		case conn := <-RequestPeers:
			fmt.Printf("requesting branches\n")

			var res = http.Response{
				Proto:      "HTTP/1.1",
				ProtoMajor: 1,
				ProtoMinor: 1,
				StatusCode: http.StatusOK,
			}

			var slice []Branch

			if branch, err := GetBranch(); err != nil {
				res.StatusCode = http.StatusInternalServerError
				err = fmt.Errorf("failed to get local branch: %+v", err)
			} else {
				slice = append(slice, branch)
				for _, branch := range branches {
					fmt.Printf("adding branch: %+v", branch)
					slice = append(slice, *branch)
				}
				if buf, err := json.Marshal(slice); err != nil {
					res.StatusCode = http.StatusInternalServerError
					err = fmt.Errorf("failed to marshal: %+v", err)
				} else {
					res.Body = io.NopCloser(bytes.NewReader(buf))
				}
			}

			go func(conn net.Conn, res http.Response) {
				if err = res.Write(conn); err != nil {
					fmt.Fprintf(os.Stderr, "error writing request: %+v", err)
				}
				conn.Close()
			}(conn, res)

		case <-ctx.Done():
			return
		}
	}
}
