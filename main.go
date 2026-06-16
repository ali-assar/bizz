// bizz is a tiny "who's on the network" tool.
//
// When it starts it finds your own IP address and your OS username, shows
// them in a window, and broadcasts them on the local network every few
// seconds. At the same time it listens for the same broadcast from anyone
// else running bizz, so everyone ends up with a live list of
// IP address <-> username for the whole office LAN. Handy for the times a
// colleague is trying to reach you (e.g. for a remote screen share) and you
// can't hear them because you've got headphones in.
//
// Build it the same way on Windows and Linux: `go build`. See README.md
// for cross-compiling instructions.
package main

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/user"
	"sort"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
)

const (
	iconFile = "assets/bizz-icon.svg"

	winWidthCompact  = 420
	winHeightCompact = 280
	winWidthExpanded = 420
	winHeightExpanded = 620

	bizzButtonSize = 140
	peerListWidth  = 380
	peerListHeight = 320
)

//go:embed assets/*
var embeddedAssets embed.FS

const (
	appPort       = 9876             // UDP port used for discovery - change if it clashes with something else on your network
	announceEvery = 3 * time.Second  // how often we tell others we're here
	peerTimeout   = 12 * time.Second // how long of silence before we drop someone from the list
	uiRefresh     = 2 * time.Second  // how often the window redraws the list

	msgAnnounce = ""
	msgBizz     = "bizz"
)

// packet is the JSON we send and receive on the LAN.
type packet struct {
	Type string `json:"type,omitempty"` // empty = presence, "bizz" = attention ping
	User string `json:"user"`
	Host string `json:"host"`
	IP   string `json:"ip"`
}

type peer struct {
	User     string
	Host     string
	IP       string
	LastSeen time.Time
}

// peerBook keeps track of everyone we've heard from recently. It's safe to
// use from multiple goroutines (the network listener writes to it, the UI
// reads from it).
type peerBook struct {
	mu   sync.Mutex
	data map[string]peer // keyed by IP
}

func newPeerBook() *peerBook {
	return &peerBook{data: make(map[string]peer)}
}

func (b *peerBook) see(p packet) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.data[p.IP] = peer{User: p.User, Host: p.Host, IP: p.IP, LastSeen: time.Now()}
}

// list returns a stable, sorted snapshot, dropping anyone who has gone
// quiet for longer than peerTimeout (they probably closed the app).
func (b *peerBook) list() []peer {
	b.mu.Lock()
	defer b.mu.Unlock()
	now := time.Now()
	out := make([]peer, 0, len(b.data))
	for ip, p := range b.data {
		if now.Sub(p.LastSeen) > peerTimeout {
			delete(b.data, ip)
			continue
		}
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].User != out[j].User {
			return out[i].User < out[j].User
		}
		return out[i].IP < out[j].IP
	})
	return out
}

// localAddress is one IPv4 on this machine, with a human-readable label.
type localAddress struct {
	IP    string
	Label string // e.g. "192.168.1.5 (wlan0)"
}

// localIPv4Addresses returns every non-loopback IPv4 on up interfaces.
func localIPv4Addresses() []localAddress {
	var out []localAddress
	ifaces, _ := net.Interfaces()
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
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
			out = append(out, localAddress{
				IP:    ip4.String(),
				Label: fmt.Sprintf("%s (%s)", ip4, iface.Name),
			})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Label < out[j].Label })
	return out
}

// defaultLocalIP picks the address the OS would use for general LAN traffic,
// or the first listed address as a fallback.
func defaultLocalIP(addrs []localAddress) string {
	if len(addrs) == 0 {
		return "127.0.0.1"
	}
	conn, err := net.Dial("udp4", "8.8.8.8:80")
	if err == nil {
		defer conn.Close()
		if a, ok := conn.LocalAddr().(*net.UDPAddr); ok && !a.IP.IsUnspecified() {
			ip := a.IP.String()
			for _, la := range addrs {
				if la.IP == ip {
					return ip
				}
			}
		}
	}
	return addrs[0].IP
}

// currentUsername returns the logged-in user's name on both Windows and
// Linux, stripping the "DOMAIN\" prefix Windows sometimes adds.
func currentUsername() string {
	if u, err := user.Current(); err == nil && u.Username != "" {
		name := u.Username
		if i := strings.LastIndex(name, "\\"); i != -1 {
			name = name[i+1:]
		}
		return name
	}
	for _, env := range []string{"USER", "USERNAME"} {
		if v := os.Getenv(env); v != "" {
			return v
		}
	}
	return "unknown"
}

type announcer struct {
	mu     sync.Mutex
	cancel context.CancelFunc
}

func (a *announcer) restart(me packet) {
	a.mu.Lock()
	if a.cancel != nil {
		a.cancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	a.cancel = cancel
	a.mu.Unlock()
	go announceLoop(ctx, me)
}

func announceLoop(ctx context.Context, me packet) {
	conn, err := net.DialUDP("udp4", nil, &net.UDPAddr{IP: net.IPv4bcast, Port: appPort})
	if err != nil {
		fmt.Println("bizz: could not open broadcast socket:", err)
		return
	}
	defer conn.Close()
	me.Type = msgAnnounce
	data, _ := json.Marshal(me)
	ticker := time.NewTicker(announceEvery)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_, _ = conn.Write(data)
		}
	}
}

func sendBizz(me packet, targetIP string) error {
	conn, err := net.DialUDP("udp4", nil, &net.UDPAddr{IP: net.ParseIP(targetIP), Port: appPort})
	if err != nil {
		return err
	}
	defer conn.Close()
	me.Type = msgBizz
	data, _ := json.Marshal(me)
	_, err = conn.Write(data)
	return err
}

// listen waits for packets from everyone else on the LAN.
func listen(book *peerBook, onBizz func(packet)) {
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4zero, Port: appPort})
	if err != nil {
		fmt.Println("bizz: could not listen for broadcasts:", err)
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
		if p.Type == msgBizz {
			if onBizz != nil {
				onBizz(p)
			}
			continue
		}
		book.see(p)
	}
}

func main() {
	host, _ := os.Hostname()
	addrs := localIPv4Addresses()
	selectedIP := defaultLocalIP(addrs)

	me := packet{User: currentUsername(), Host: host, IP: selectedIP}
	book := newPeerBook()
	book.see(me)

	var ann announcer
	ann.restart(me)

	a := app.NewWithID("io.bizz.app")
	a.Settings().SetTheme(newBizzTheme())

	iconSVG, _ := embeddedAssets.ReadFile(iconFile)
	icon := rasterizeSVG("bizz-icon.png", iconSVG, 256)
	a.SetIcon(icon)

	w := a.NewWindow("bizz")
	w.SetIcon(icon)
	w.Resize(fyne.NewSize(winWidthCompact, winHeightCompact))
	w.SetFixedSize(false)
	w.CenterOnScreen()

	var panelOpen bool
	var lastPeerSig string
	selected := make(map[string]bool)
	checks := make(map[string]*widget.Check)

	onBizzReceived := func(from packet) {
		fyne.Do(func() {
			dialog.ShowInformation(
				"Bizz!",
				fmt.Sprintf("%s on %s (%s) wants your attention.", from.User, from.Host, from.IP),
				w,
			)
			w.RequestFocus()
		})
	}

	go listen(book, onBizzReceived)

	// --- IP picker ---
	ipLabels := make([]string, len(addrs))
	labelToIP := make(map[string]string, len(addrs))
	for i, la := range addrs {
		ipLabels[i] = la.Label
		labelToIP[la.Label] = la.IP
	}
	if len(ipLabels) == 0 {
		ipLabels = []string{"127.0.0.1 (loopback)"}
		labelToIP["127.0.0.1 (loopback)"] = "127.0.0.1"
	}

	selectedLabel := ""
	for _, la := range addrs {
		if la.IP == selectedIP {
			selectedLabel = la.Label
			break
		}
	}
	if selectedLabel == "" && len(ipLabels) > 0 {
		selectedLabel = ipLabels[0]
		selectedIP = labelToIP[selectedLabel]
		me.IP = selectedIP
	}

	ipSelect := widget.NewSelect(ipLabels, func(label string) {
		selectedIP = labelToIP[label]
		me.IP = selectedIP
		book.see(me)
		ann.restart(me)
	})
	ipSelect.SetSelected(selectedLabel)
	ipSelect.PlaceHolder = "Choose network interface"

	title := canvas.NewText("bizz", colorAmberGlow)
	title.TextStyle = fyne.TextStyle{Bold: true}
	title.TextSize = 24
	title.Alignment = fyne.TextAlignCenter

	youLabel := widget.NewLabel("")
	youLabel.Alignment = fyne.TextAlignCenter
	refreshYouLabel := func() {
		youLabel.SetText(fmt.Sprintf("You: %s  ·  %s", me.User, me.IP))
	}
	refreshYouLabel()

	netLabel := widget.NewLabelWithStyle("Network", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})

	// --- peer checklist (shown after tapping the bizz button) ---
	peerChecks := container.NewVBox()
	peerScroll := container.NewVScroll(peerChecks)
	peerScroll.SetMinSize(fyne.NewSize(peerListWidth, peerListHeight))

	rebuildPeerChecks := func() {
		peers := book.list()
		sig := peerSig(peers, me.IP)
		if sig == lastPeerSig && len(peerChecks.Objects) > 0 {
			return
		}
		lastPeerSig = sig

		peerChecks.Objects = nil
		checks = make(map[string]*widget.Check)
		for _, p := range peers {
			if p.IP == me.IP {
				continue
			}
			peer := p
			label := fmt.Sprintf("%s · %s (%s)", peer.User, peer.Host, peer.IP)
			checked := selected[peer.IP]
			chk := widget.NewCheck(label, func(on bool) {
				if on {
					selected[peer.IP] = true
				} else {
					delete(selected, peer.IP)
				}
			})
			chk.SetChecked(checked)
			checks[peer.IP] = chk
			peerChecks.Add(chk)
		}
		if len(peerChecks.Objects) == 0 {
			peerChecks.Add(widget.NewLabel("Nobody else on the LAN yet…"))
		}
		peerChecks.Refresh()
	}

	bizzSelectedBtn := widget.NewButton("Bizz selected", func() {
		if len(selected) == 0 {
			dialog.ShowInformation("Bizz", "Select at least one person to bizz.", w)
			return
		}
		sent := 0
		for ip := range selected {
			if err := sendBizz(me, ip); err == nil {
				sent++
			}
		}
		dialog.ShowInformation("Bizz", fmt.Sprintf("Bizzed %d colleague(s).", sent), w)
	})
	bizzSelectedBtn.Importance = widget.HighImportance

	userPanel := container.NewVBox(
		widget.NewSeparator(),
		widget.NewLabelWithStyle("Who do you want to bizz?", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		peerScroll,
		bizzSelectedBtn,
	)
	userPanel.Hide()

	togglePanel := func() {
		panelOpen = !panelOpen
		if panelOpen {
			rebuildPeerChecks()
			userPanel.Show()
			w.Resize(fyne.NewSize(winWidthExpanded, winHeightExpanded))
		} else {
			userPanel.Hide()
			w.Resize(fyne.NewSize(winWidthCompact, winHeightCompact))
		}
	}

	bizzBtn := newTappableIcon(icon, bizzButtonSize, togglePanel)

	hint := widget.NewLabelWithStyle("Tap the bee to pick colleagues", fyne.TextAlignCenter, fyne.TextStyle{Italic: true})

	compact := container.NewVBox(
		title,
		youLabel,
		widget.NewSeparator(),
		netLabel,
		ipSelect,
		container.NewCenter(bizzBtn),
		hint,
	)

	bg := canvas.NewRectangle(colorInk)
	bg.FillColor = colorInk
	content := container.NewStack(
		bg,
		container.NewPadded(container.NewVBox(compact, userPanel)),
	)
	w.SetContent(content)

	go func() {
		ticker := time.NewTicker(uiRefresh)
		defer ticker.Stop()
		for range ticker.C {
			fyne.Do(func() {
				if panelOpen {
					rebuildPeerChecks()
				}
				refreshYouLabel()
			})
		}
	}()

	w.ShowAndRun()
}

func peerSig(peers []peer, myIP string) string {
	var b strings.Builder
	for _, p := range peers {
		if p.IP == myIP {
			continue
		}
		b.WriteString(p.IP)
		b.WriteByte('|')
		b.WriteString(p.User)
		b.WriteByte('|')
	}
	return b.String()
}
