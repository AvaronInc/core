package wireguard

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"log"
	"net"
	"os/exec"
	"strconv"
	"strings"
)

type Key [32]byte

type Peer struct {
	PresharedKey        *Key         `json:"presharedKey"`
	Endpoint            string       `json:"endpoint"`
	AllowedIPs          []*net.IPNet `json:"allowedIPs"`
	LatestHandshake     string       `json:"latestHandshake"`
	Received            string    `json:"received"`
	Sent                string    `json:"sent"`
	PersistentKeepalive string       `json:"persistentKeepalive"`
}

type Interface struct {
	Name          string        `json:"name"`
	PrivateKey    *Key          `json:"privateKey"`
	ListeningPort int           `json:"listeningPort"`
	Peers         map[Key]*Peer `json:"peers"`
}

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

func (k Key) Path() string {
	return strings.Replace(k.String(), "/", "-", -1)
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

func Interfaces(ctx context.Context) (map[Key]*Interface, error) {
	var (
		m    = make(map[Key]*Interface)
		i    *Interface
		peer *Peer
		cmd  = exec.CommandContext(ctx, "sudo", "/bin/wg")
		key  Key
	)

	r, err := cmd.StdoutPipe()
	if err != nil {
		return m, err
	}

	if err := cmd.Start(); err != nil {
		return m, err
	}

	scanner := bufio.NewScanner(r)

	const (
		StateNone int = iota
		StateInterface
		StatePeer
	)

	state := StateNone

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			state = StateNone
			continue
		}

		j := strings.Index(line, ":")
		if j == -1 {
			err = fmt.Errorf("failed to find ':' in wg output: %s", line)
			break
		}

		k, v := line[:j], strings.TrimSpace(line[j+1:])
		log.Println("key:", k, "value:", v)

		switch state {
		case StateNone:
			switch k {
			case "interface":
				state = StateInterface
				i = &Interface{
					Peers: make(map[Key]*Peer),
					Name:  v,
				}
			case "peer":
				state = StatePeer
				if i == nil {
					return m, fmt.Errorf("unexpected 'peer': %s", line)
				}
				peer = &Peer{}
				if _, err := key.UnmarshalText([]byte(v)); err != nil {
					return m, err
				}
				i.Peers[key] = peer
				log.Println("decoded peer")
			default:
				return m, fmt.Errorf("failed to match state in wg output: %s", line)
			}
		case StateInterface:
			switch k {
			case "public key":
				var key Key
				if _, err := key.UnmarshalText([]byte(v)); err != nil {
					return m, err
				}
				m[key] = i
			case "private key":
				var key Key
				if v == "(hidden)" {
					// ok
				} else if _, err := key.UnmarshalText([]byte(v)); err != nil {
					return m, err
				} else {
					i.PrivateKey = &key
				}
			case "listening port":
				if i.ListeningPort, err = strconv.Atoi(v); err != nil {
					return m, err
				}
			}
		case StatePeer:
			switch k {
			case "preshared key":
				var key Key
				if v == "(hidden)" {
					// ok
				} else if _, err := key.UnmarshalText([]byte(v)); err != nil {
					return m, err
				} else {
					peer.PresharedKey = &key
				}
			case "endpoint":
				peer.Endpoint = v
			case "allowed ips":
				for _, s := range strings.Split(v, ", ") {
					log.Println("parsing CIDR", s)
					_, net, err := net.ParseCIDR(s)
					if err != nil {
						return m, err
					}
					peer.AllowedIPs = append(peer.AllowedIPs, net)
				}
			case "latest handshake":
				peer.LatestHandshake = v
			case "transfer":
				w := strings.Split(v, ", ")
				if len(w) < 2 {
					return m, fmt.Errorf("expected comma in transfer(tx, rx): %s", line)
				}
				peer.Received = strings.TrimSuffix(w[0], " received")
				peer.Sent = strings.TrimSuffix(w[1], " sent")
			case "persistent keepalive":
				peer.PersistentKeepalive = v
			}
		}

	}

	if err := scanner.Err(); err != nil {
		return m, err
	}

	return m, cmd.Wait()
}

/*
interface: wg0
  public key: IY/C7eZfk3/YJbiExUQY39zMjPqn77sXoKUWKm70Vw4=
  private key: (hidden)
  listening port: 49544

peer: h7HfpSlMu/99KnouS6s8Ugcmemmw2rvND9jrwTvv7UE=
  preshared key: (hidden)
  endpoint: 45.77.215.144:51820
  allowed ips: 10.0.0.0/24
  latest handshake: 1 minute, 27 seconds ago
  transfer: 2.47 GiB received, 54.44 MiB sent
  persistent keepalive: every 25 seconds

interface: avaron
  public key: gnH2O6at5ezSKaUezd/c1FpeO8gtYdRXtpo1Km/nxXg=
  private key: (hidden)
  listening port: 51820

*/
