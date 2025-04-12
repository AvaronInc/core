package main

import (
	"bufio"
	"bytes"
	"fmt"
	nl "github.com/vishvananda/netlink"
	"net"
	"net/http"
	"os"
	"sort"
	"time"
	"os/exec"
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
	reader := bufio.NewReader(conn)
	req, err := http.ReadRequest(reader)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error reading request: %+v\n", err)
		return
	}

	fmt.Fprintf(os.Stderr, "Method: %s\n", req.Method)
	fmt.Fprintf(os.Stderr, "URL: %s\n", req.URL)
	fmt.Fprintf(os.Stderr, "Headers: %v\n", req.Header)

	response := "HTTP/1.1 200 OK\r\nContent-Type: text/plain\r\n\r\nHello, World!\r\n"
	_, err = conn.Write([]byte(response))
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

func startup() error {
	cmd := exec.Command("prog")
	if err := cmd.Run(); err != nil {
		return err
	}
	return nil
}

func main() {
	links, err := nl.LinkList()
	if err != nil {
		fmt.Println("error getting interfaces:", err)
		os.Exit(1)
	}

	for _, link := range links {
		attrs := link.Attrs()
		fmt.Fprintf(os.Stderr, "name: %s, mac: %s, flags: %s\n", attrs.Name, attrs.HardwareAddr, attrs.Flags)
	}

	routes, err := nl.RouteList(nil, nl.FAMILY_V4)
	if err != nil {
		fmt.Println("error getting routes: %+v", err)
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

	conns := make(chan net.Conn)

	for i := 0; i < 8; i++ {
		go func() {
			for conn := range conns {
				conn.SetDeadline(time.Now().Add(time.Second * 10))
				handle(conn)
				conn.Close()
			}
		}()
	}

	for {
		conn, err := listener.Accept()
		if err != nil {
			fmt.Fprintf(os.Stderr, "error accepting connection: %+v\n", err)
			continue
		}
		conns <- conn
	}
}
