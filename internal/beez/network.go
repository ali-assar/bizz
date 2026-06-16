package beez

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"sort"
	"strings"
	"time"
)

// localAddress is one IPv4 on this machine.
type localAddress struct {
	IP        string
	Iface     string
	Label     string
	Broadcast string // subnet-directed broadcast, e.g. 192.168.1.255
	VPN       bool
}

func isVPNInterface(name string) bool {
	n := strings.ToLower(name)
	for _, hint := range []string{
		"tun", "tap", "wg", "utun", "ppp", "nordlynx", "openvpn",
		"wireguard", "vpn", "zerotier", "zt", "tailscale", "ts",
		"hamachi", "nordvpn", "proton", "cisco", "anyconnect",
	} {
		if strings.Contains(n, hint) {
			return true
		}
	}
	return false
}

func isPrivateIPv4(ip net.IP) bool {
	ip = ip.To4()
	if ip == nil {
		return false
	}
	return ip[0] == 10 ||
		(ip[0] == 172 && ip[1] >= 16 && ip[1] <= 31) ||
		(ip[0] == 192 && ip[1] == 168)
}

func subnetBroadcast(ipNet *net.IPNet) net.IP {
	ip := ipNet.IP.To4()
	if ip == nil {
		return nil
	}
	ones, bits := ipNet.Mask.Size()
	if bits != 32 || ones >= 31 {
		return nil
	}
	mask := ipNet.Mask
	if len(mask) == 16 {
		mask = mask[12:16]
	}
	if len(mask) != 4 {
		return nil
	}
	bcast := make(net.IP, 4)
	for i := range ip {
		bcast[i] = ip[i] | ^mask[i]
	}
	return bcast
}

// localIPv4Addresses returns every non-loopback IPv4 on up interfaces.
func localIPv4Addresses() []localAddress {
	var out []localAddress
	ifaces, _ := net.Interfaces()
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		vpn := isVPNInterface(iface.Name)
		addrs, _ := iface.Addrs()
		for _, addr := range addrs {
			ipNet, ok := addr.(*net.IPNet)
			if !ok {
				continue
			}
			ip4 := ipNet.IP.To4()
			if ip4 == nil {
				continue
			}
			bcast := subnetBroadcast(ipNet)
			label := fmt.Sprintf("%s (%s)", ip4, iface.Name)
			if vpn {
				label += " · VPN"
			}
			la := localAddress{
				IP:    ip4.String(),
				Iface: iface.Name,
				Label: label,
				VPN:   vpn,
			}
			if bcast != nil {
				la.Broadcast = bcast.String()
			}
			out = append(out, la)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Label < out[j].Label })
	return out
}

// defaultLocalIP prefers a plain LAN address over VPN virtual adapters.
func defaultLocalIP(addrs []localAddress) string {
	if len(addrs) == 0 {
		return "127.0.0.1"
	}
	bestScore := -1
	bestIP := addrs[0].IP
	for _, la := range addrs {
		score := 0
		if isPrivateIPv4(net.ParseIP(la.IP)) {
			score += 4
		}
		if !la.VPN {
			score += 8
		}
		if strings.HasPrefix(la.IP, "192.168.") {
			score += 2
		}
		if score > bestScore {
			bestScore = score
			bestIP = la.IP
		}
	}
	return bestIP
}

func findLocalAddress(addrs []localAddress, ip string) (localAddress, bool) {
	for _, la := range addrs {
		if la.IP == ip {
			return la, true
		}
	}
	return localAddress{}, false
}

func broadcastTargets(la localAddress) []*net.UDPAddr {
	seen := make(map[string]bool)
	var out []*net.UDPAddr
	add := func(ip string) {
		if ip == "" || seen[ip] {
			return
		}
		seen[ip] = true
		out = append(out, &net.UDPAddr{IP: net.ParseIP(ip), Port: appPort})
	}
	// Directed subnet broadcast is what Windows reliably hears.
	add(la.Broadcast)
	// Global broadcast as a fallback on Linux / some switches.
	add(net.IPv4bcast.String())
	return out
}

func announceConn(localIP string) (*net.UDPConn, error) {
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.ParseIP(localIP), Port: 0})
	if err != nil {
		return nil, err
	}
	if err := enableBroadcast(conn); err != nil {
		conn.Close()
		return nil, err
	}
	return conn, nil
}

func dialUDPFrom(localIP, remoteIP string, remotePort int) (*net.UDPConn, error) {
	local := &net.UDPAddr{IP: net.ParseIP(localIP), Port: 0}
	remote := &net.UDPAddr{IP: net.ParseIP(remoteIP), Port: remotePort}
	conn, err := net.DialUDP("udp4", local, remote)
	if err != nil {
		return nil, err
	}
	if err := enableBroadcast(conn); err != nil {
		return nil, err
	}
	return conn, nil
}

func announceLoop(ctx context.Context, me packet, la localAddress) {
	conn, err := announceConn(me.IP)
	if err != nil {
		fmt.Println("beez: could not open announce socket on", me.IP+":", err)
		return
	}
	defer conn.Close()

	targets := broadcastTargets(la)
	me.Type = msgAnnounce
	data, _ := json.Marshal(me)

	ticker := time.NewTicker(announceEvery)
	defer ticker.Stop()

	send := func() {
		for _, addr := range targets {
			_, _ = conn.WriteToUDP(data, addr)
		}
	}
	send() // announce immediately on start / network change

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			send()
		}
	}
}

func sendbeez(me packet, la localAddress, targetIP, reason string) error {
	conn, err := dialUDPFrom(me.IP, targetIP, appPort)
	if err != nil {
		return err
	}
	defer conn.Close()
	me.Type = msgbeez
	me.Reason = truncatebeezReason(reason)
	data, _ := json.Marshal(me)
	_, err = conn.Write(data)
	return err
}

func listen(book *peerBook, onbeez func(packet)) {
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4zero, Port: appPort})
	if err != nil {
		fmt.Println("beez: could not listen for broadcasts:", err)
		return
	}
	defer conn.Close()
	buf := make([]byte, 512)
	for {
		n, _, err := conn.ReadFromUDP(buf)
		if err != nil {
			continue
		}
		var p packet
		if json.Unmarshal(buf[:n], &p) != nil || p.IP == "" {
			continue
		}
		if p.Type == msgbeez {
			if onbeez != nil {
				onbeez(p)
			}
			continue
		}
		book.see(p)
	}
}
