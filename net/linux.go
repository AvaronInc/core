package netlink

import (
	"bufio"
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"strconv"
)

type Netlink struct {
	IfIndex        int        `json:"ifindex"`
	IfName         string     `json:"ifname"`
	Flags          []string   `json:"flags"`
	MTU            int        `json:"mtu"`
	Qdisc          string     `json:"qdisc"`
	OperState      string     `json:"operstate"`
	Group          string     `json:"group"`
	TxQlen         int        `json:"txqlen"`
	LinkType       string     `json:"link_type"`
	Address        string     `json:"address"`
	Broadcast      string     `json:"broadcast"`
	Promiscuity    int        `json:"promiscuity"`
	AllMulti       int        `json:"allmulti"`
	MinMTU         int        `json:"min_mtu"`
	MaxMTU         int        `json:"max_mtu"`
	NumTXQueues    int        `json:"num_tx_queues"`
	NumRXQueues    int        `json:"num_rx_queues"`
	GSOMaxSize     int        `json:"gso_max_size"`
	GSOMaxSegs     int        `json:"gso_max_segs"`
	TSOMaxSize     int        `json:"tso_max_size"`
	TSOMaxSegs     int        `json:"tso_max_segs"`
	GROMaxSize     int        `json:"gro_max_size"`
	GSOIPv4MaxSize int        `json:"gso_ipv4_max_size"`
	GROIPv4MaxSize int        `json:"gro_ipv4_max_size"`
	AddrInfo       []AddrInfo `json:"addr_info"`
	Stats64        Stats64    `json:"stats64"`
	ParentBus      string     `json:"parentbus,omitempty"`
	ParentDev      string     `json:"parentdev,omitempty"`
	AltNames       []string   `json:"altnames,omitempty"`
	LinkInfo       *LinkInfo  `json:"linkinfo,omitempty"`
}

type AddrInfo struct {
	Family            string `json:"family"`
	Local             string `json:"local"`
	PrefixLen         int    `json:"prefixlen"`
	Broadcast         string `json:"broadcast,omitempty"`
	Scope             string `json:"scope"`
	Label             string `json:"label,omitempty"`
	ValidLifeTime     uint64 `json:"valid_life_time"`
	PreferredLifeTime uint64 `json:"preferred_life_time"`
	Dynamic           bool   `json:"dynamic,omitempty"`
	NoPrefixRoute     bool   `json:"noprefixroute,omitempty"`
}

type Stats64 struct {
	Rx Stats `json:"rx"`
	Tx Stats `json:"tx"`
}

type Stats struct {
	Bytes         uint64 `json:"bytes"`
	Packets       uint64 `json:"packets"`
	Errors        uint64 `json:"errors"`
	Dropped       uint64 `json:"dropped"`
	OverErrors    uint64 `json:"over_errors"`
	Multicast     uint64 `json:"multicast"`
	CarrierErrors uint64 `json:"carrier_errors,omitempty"`
	Collisions    uint64 `json:"collisions,omitempty"`
}

type TCPMetric struct {
	Destination           net.IP  `json:"dst"`
	Source                net.IP  `json:"source"`
	Age                   float64 `json:"age"`
	CongestionWindow      uint64  `json:"cwnd"`
	RoundTripTime         float64 `json:"rtt"`
	RoundTripTimeVariance float64 `json:"rttvar"`
}

type LinkInfo struct {
	InfoKind string        `json:"info_kind"`
	InfoData *LinkInfoData `json:"info_data"`
}

type LinkInfoData struct {
	ForwardDelay            int     `json:"forward_delay"`
	HelloTime               int     `json:"hello_time"`
	MaxAge                  int     `json:"max_age"`
	AgeingTime              int     `json:"ageing_time"`
	StpState                int     `json:"stp_state"`
	Priority                int     `json:"priority"`
	VlanFiltering           int     `json:"vlan_filtering"`
	VlanProtocol            string  `json:"vlan_protocol"`
	BridgeID                string  `json:"bridge_id"`
	RootID                  string  `json:"root_id"`
	RootPort                int     `json:"root_port"`
	RootPathCost            int     `json:"root_path_cost"`
	TopologyChange          int     `json:"topology_change"`
	TopologyChangeDetected  int     `json:"topology_change_detected"`
	HelloTimer              float64 `json:"hello_timer"`
	TcnTimer                float64 `json:"tcn_timer"`
	TopologyChangeTimer     float64 `json:"topology_change_timer"`
	GcTimer                 float64 `json:"gc_timer"`
	FdbNlearned             int     `json:"fdb_n_learned"`
	FdbMaxLearned           int     `json:"fdb_max_learned"`
	VlanDefaultPvid         int     `json:"vlan_default_pvid"`
	VlanStatsEnabled        int     `json:"vlan_stats_enabled"`
	VlanStatsPerPort        int     `json:"vlan_stats_per_port"`
	GroupFwdMask            string  `json:"group_fwd_mask"`
	GroupAddr               string  `json:"group_addr"`
	McastSnooping           int     `json:"mcast_snooping"`
	NoLinkLocalLearn        int     `json:"no_linklocal_learn"`
	McastVlanSnooping       int     `json:"mcast_vlan_snooping"`
	MstEnabled              int     `json:"mst_enabled"`
	McastRouter             int     `json:"mcast_router"`
	McastQueryUseIfaddr     int     `json:"mcast_query_use_ifaddr"`
	McastQuerier            int     `json:"mcast_querier"`
	McastHashElasticity     int     `json:"mcast_hash_elasticity"`
	McastHashMax            int     `json:"mcast_hash_max"`
	McastLastMemberCnt      int     `json:"mcast_last_member_cnt"`
	McastStartupQueryCnt    int     `json:"mcast_startup_query_cnt"`
	McastLastMemberIntvl    int     `json:"mcast_last_member_intvl"`
	McastMembershipIntvl    int     `json:"mcast_membership_intvl"`
	McastQuerierIntvl       int     `json:"mcast_querier_intvl"`
	McastQueryIntvl         int     `json:"mcast_query_intvl"`
	McastQueryResponseIntvl int     `json:"mcast_query_response_intvl"`
	McastStartupQueryIntvl  int     `json:"mcast_startup_query_intvl"`
	McastStatsEnabled       int     `json:"mcast_stats_enabled"`
	McastIgmpVersion        int     `json:"mcast_igmp_version"`
	McastMldVersion         int     `json:"mcast_mld_version"`
	NfCallIptables          int     `json:"nf_call_iptables"`
	NfCallIp6tables         int     `json:"nf_call_ip6tables"`
	NfCallArptables         int     `json:"nf_call_arptables"`
}

type Ethtool struct {
	//IfName                               string   `json:"ifname"`
	SupportedPorts                       []string `json:"supported-ports"`
	SupportedLinkModes                   []string `json:"supported-link-modes"`
	SupportedPauseFrameUse               string   `json:"supported-pause-frame-use"`
	SupportsAutoNegotiation              bool     `json:"supports-auto-negotiation"`
	SupportedFecModes                    []string `json:"supported-fec-modes"`
	AdvertisedLinkModes                  []string `json:"advertised-link-modes"`
	AdvertisedPauseFrameUse              string   `json:"advertised-pause-frame-use"`
	AdvertisedAutoNegotiation            bool     `json:"advertised-auto-negotiation"`
	AdvertisedFecModes                   []string `json:"advertised-fec-modes"`
	LinkPartnerAdvertisedLinkModes       []string `json:"link-partner-advertised-link-modes"`
	LinkPartnerAdvertisedPauseFrameUse   string   `json:"link-partner-advertised-pause-frame-use"`
	LinkPartnerAdvertisedAutoNegotiation bool     `json:"link-partner-advertised-auto-negotiation"`
	LinkPartnerAdvertisedFecModes        []string `json:"link-partner-advertised-fec-modes"`
	Speed                                int      `json:"speed"`
	Duplex                               string   `json:"duplex"`
	AutoNegotiation                      bool     `json:"auto-negotiation"`
	Port                                 string   `json:"port"`
	PhyAd                                int      `json:"phyad"`
	Transceiver                          string   `json:"transceiver"`
	MdiX                                 bool     `json:"mdi-x"`
	MdiXForced                           bool     `json:"mdi-x-forced"`
	MdiXAuto                             bool     `json:"mdi-x-auto"`
	SupportsWakeOn                       string   `json:"supports-wake-on"`
	WakeOn                               string   `json:"wake-on"`
	CurrentMessageLevel                  int      `json:"current-message-level"`
	LinkDetected                         bool     `json:"link-detected"`
}

type Interface struct {
	Ethtool
	Netlink
}

type Route struct {
	Type        string
	Destination net.IPNet
	Gateway     net.IP
	Interface   string
	Protocol    string
	Scope       string
	Source      string
	Metric      int
	Flags       []string
}

func (i *Interface) IPs() []net.IP {
	var ips []net.IP

	m := make(map[string]struct{})
	for _, addr := range i.AddrInfo {
		ip := net.ParseIP(addr.Local)
		_, exists := m[string(ip)]
		if exists {
			continue
		}
		ips = append(ips, ip)
	}
	return ips
}

func (r *Route) IP() net.IP {
	return r.Gateway
}

func (r *Route) IPNet() *net.IPNet {
	return &r.Destination
}

func (a *AddrInfo) IPNet() *net.IPNet {
	str := fmt.Sprintf("%s/%d", a.Local, a.PrefixLen)
	_, n, err := net.ParseCIDR(str)
	if err != nil {
		return &net.IPNet{}
	}
	return n
}

type RouteMask []Route

func (r RouteMask) Len() int {
	return len(r)
}
func (r RouteMask) Swap(i, j int) {
	r[i], r[j] = r[j], r[i]
}
func (r RouteMask) Less(i, j int) bool {
	if !bytes.Equal(r[i].IPNet().Mask, r[j].IPNet().Mask) {
		return bytes.Compare(r[i].IPNet().Mask, r[j].IPNet().Mask) < 0
	}

	return r[i].Metric < r[j].Metric
}

type AddressMask []AddrInfo

func (a AddressMask) Len() int {
	return len(a)
}

func (a AddressMask) Swap(i, j int) {
	a[i], a[j] = a[j], a[i]
}

func (a AddressMask) Less(i, j int) bool {
	ib, il := a[i].IPNet().Mask.Size()
	jb, jl := a[j].IPNet().Mask.Size()

	var b bool

	// if one address is 32-bit(IPv4) & the other is 128-bit(IPv6), multiply by 4
	if il < jl {
		ib *= (jl / il)
	} else if jl < il {
		jb *= (il / jl)
		b = true
	}

	if ib == jb {
		return b
	}
	return ib < jb

}

func Metrics(ctx context.Context) (metrics []TCPMetric, err error) {
	cmd := exec.CommandContext(ctx, "sudo", "/usr/sbin/ip", "-json", "-s", "-d", "tcpmetrics")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return metrics, err
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return metrics, err
	}

	dec := json.NewDecoder(stdout)
	err = cmd.Start()
	if err != nil {
		buf, _ := io.ReadAll(stderr)
		return metrics, fmt.Errorf("error ip tcmetrics output: %+v - stderr: %s", err, string(buf))
	}

	if t, _ := dec.Token(); t != json.Delim('[') {
		return metrics, fmt.Errorf("expected '[' as starting delimeter for `ip tcpmetrics`")
	}

	var metric TCPMetric
	for dec.More() {
		err = dec.Decode(&metric)
		if err != nil {
			break
		}
		metrics = append(metrics, metric)
	}
	if err != nil {
		return metrics, err
	}

	if t, _ := dec.Token(); t != json.Delim(']') {
		return metrics, fmt.Errorf("expected ']' as closing delimeter for tcpmetrics")
	}
	return metrics, nil
}

func Routes(ctx context.Context) (routes []Route, err error) {
	file, err := os.Open("/sys/class/net/route")
	if err != nil {
		return nil, err
	}

	dec := func(src []byte) []byte {
		dst := make([]byte, len(src)/2)
		if _, err := hex.Decode(dst, src); err != nil {
			panic(err)
		}
		return dst
	}

	atoi := func(buf []byte) int {
		if n, err := strconv.Atoi(string(buf)); err != nil {
			panic(err)
		} else {
			return n
		}
	}

	scanner := bufio.NewScanner(file)
	for i := 0; scanner.Scan(); i++ {
		line := scanner.Bytes()
		if i == 0 || len(line) == 0 {
			continue
		}

		fields := bytes.Fields(line)
		if len(fields) < 11 {
			continue
		}

		// Iface Destination Gateway Flags RefCnt Use Metric Mask MTU Window IRTT
		route := Route{
			Destination: net.IPNet{
				IP:   dec(fields[1]),
				Mask: dec(fields[7]),
			},
			Gateway:   net.IP(dec(fields[2])),
			Interface: string(fields[0]),
			Metric:    atoi(fields[6]),
		}
		routes = append(routes, route)
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return

}

func runEthtool(ctx context.Context, dst *Ethtool, device string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "sudo", "/usr/sbin/ethtool", "--json", device)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}

	dec := json.NewDecoder(stdout)
	err = cmd.Start()
	if err != nil {
		return nil, err
	}

	if t, _ := dec.Token(); t != json.Delim('[') {
		return nil, fmt.Errorf("expected '[' as starting delimeter")
	}

	err = dec.Decode(dst)
	if err != nil {
		goto fail
	}

	if t, _ := dec.Token(); t != json.Delim(']') {
		return nil, fmt.Errorf("expected ']' as ending delimeter")
	}

	err = cmd.Wait()

fail:
	buf, _ := io.ReadAll(stderr)
	cmd.Wait()
	return buf, err
}

func List(ctx context.Context) (m map[string]*Interface, err error) {
	cmd := exec.CommandContext(ctx, "sudo", "/usr/sbin/ip", "-json", "-s", "-d", "address", "show")
	stdout, err := cmd.StdoutPipe()

	m = make(map[string]*Interface)
	if err != nil {
		return m, err
	}

	dec := json.NewDecoder(stdout)
	err = cmd.Start()
	if err != nil {
		return m, err
	}

	if t, _ := dec.Token(); t != json.Delim('[') {
		return m, fmt.Errorf("expected '[' as starting delimeter for `ip address show` output")
	}

	for dec.More() {
		i := new(Interface)
		err := dec.Decode(&i.Netlink)
		if err != nil {
			return m, err
		}

		m[i.Netlink.IfName] = i
		if i.Netlink.IfName == "lo" || i.Netlink.LinkType != "ether" {
			continue
		}

		buf, err := runEthtool(ctx, &i.Ethtool, i.Netlink.IfName)
		if err != nil && buf == nil {
			return m, fmt.Errorf("error decoding ethtool output given interface '%s': %+v", i.Netlink.IfName, err)
		} else if err != nil {
			return m, fmt.Errorf("error decoding ethtool output given interface '%s': %+v - stderr: %s", i.Netlink.IfName, err, string(buf))
		}
	}

	if t, _ := dec.Token(); t != json.Delim(']') {
		return m, fmt.Errorf("expected ']' as closing delimeter")
	}

	return m, nil
}

func ListBrief(ctx context.Context, w io.WriteCloser) (err error) {
	defer w.Close()

	cmd := exec.CommandContext(ctx, "sudo", "/usr/sbin/ip", "-br", "address", "show")
	cmd.Stdout = w
	return cmd.Run()
}
