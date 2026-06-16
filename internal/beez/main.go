// beez is a tiny "who's on the network" tool.
//
// When it starts it finds your own IP address and your OS username, shows
// them in a window, and broadcasts them on the local network every few
// seconds. At the same time it listens for the same broadcast from anyone
// else running beez, so everyone ends up with a live list of
// IP address <-> username for the whole office LAN. Handy for the times a
// colleague is trying to reach you (e.g. for a remote screen share) and you
// can't hear them because you've got headphones in.
//
// Build it the same way on Windows and Linux: `go build`. See README.md
// for cross-compiling instructions.
package beez

import (
	"beez/internal/audio"
	"context"
	"embed"
	"fmt"
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
	"fyne.io/fyne/v2/widget"
)

const (
	iconFile = "assets/beez-icon.svg"
	iconPNG  = "assets/beez-icon.png"

	winWidthCompact   = 280
	winHeightCompact  = 210
	winWidthExpanded  = 280
	winHeightExpanded = 365

	beezButtonSize = 72
	peerListWidth  = 248
	peerListHeight = 100

	maxbeezReason = 200

	// Twelve quick-pick emojis in a 6-column grid under the reason field.
	beezEmojis = "👀😊😠👌😂😢🤔👍👎🥳😭❤️"
)

//go:embed assets/*
var embeddedAssets embed.FS

const (
	appPort       = 9876             // UDP port used for discovery - change if it clashes with something else on your network
	announceEvery = 3 * time.Second  // how often we tell others we're here
	peerTimeout   = 12 * time.Second // how long of silence before we drop someone from the list
	uiRefresh     = 2 * time.Second  // how often the window redraws the list

	msgAnnounce = ""
	msgbeez     = "beez"
)

// packet is the JSON we send and receive on the LAN.
type packet struct {
	Type   string `json:"type,omitempty"` // empty = presence, "beez" = attention ping
	User   string `json:"user"`
	Host   string `json:"host"`
	IP     string `json:"ip"`
	Reason string `json:"reason,omitempty"` // why they beezed (beez packets only)
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

func (a *announcer) restart(me packet, la localAddress) {
	a.mu.Lock()
	if a.cancel != nil {
		a.cancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	a.cancel = cancel
	a.mu.Unlock()
	go announceLoop(ctx, me, la)
}

func truncatebeezReason(s string) string {
	s = strings.TrimSpace(s)
	runes := []rune(s)
	if len(runes) <= maxbeezReason {
		return s
	}
	return string(runes[:maxbeezReason])
}

func beezNotificationContent(from packet) string {
	who := fmt.Sprintf("%s on %s", from.User, from.Host)
	if r := strings.TrimSpace(from.Reason); r != "" {
		return fmt.Sprintf("%s — %s", who, r)
	}
	return fmt.Sprintf("%s wants your attention.", who)
}

func Run() {
	host, _ := os.Hostname()
	addrs := localIPv4Addresses()
	selectedIP := defaultLocalIP(addrs)

	me := packet{User: currentUsername(), Host: host, IP: selectedIP}
	selectedLA, _ := findLocalAddress(addrs, selectedIP)
	book := newPeerBook()
	book.see(me)

	var ann announcer
	ann.restart(me, selectedLA)

	a := app.NewWithID("io.beez.app")
	a.Settings().SetTheme(newbeezTheme())

	icon := loadAppIcon(embeddedAssets, iconPNG, iconFile)
	a.SetIcon(icon)

	w := a.NewWindow("beez")
	w.SetIcon(icon)
	w.Resize(fyne.NewSize(winWidthCompact, winHeightCompact))
	w.SetFixedSize(false)
	w.CenterOnScreen()

	var panelOpen bool
	var lastPeerSig string
	selected := make(map[string]bool)
	checks := make(map[string]*widget.Check)

	onbeezReceived := func(from packet) {
		fyne.Do(func() {
			audio.PlayBeeNotification()
			a.SendNotification(&fyne.Notification{
				Title:   "Beez!",
				Content: beezNotificationContent(from),
			})
			w.RequestFocus()
		})
	}

	go listen(book, onbeezReceived)

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
		if la, ok := findLocalAddress(addrs, selectedIP); ok {
			selectedLA = la
		}
		book.see(me)
		ann.restart(me, selectedLA)
	})
	ipSelect.SetSelected(selectedLabel)
	ipSelect.PlaceHolder = "Choose network interface"

	title := canvas.NewText("beez", colorAmberGlow)
	title.TextStyle = fyne.TextStyle{Bold: true}
	title.TextSize = 18
	title.Alignment = fyne.TextAlignCenter

	youLabel := widget.NewLabel("")
	youLabel.Alignment = fyne.TextAlignCenter
	refreshYouLabel := func() {
		youLabel.SetText(fmt.Sprintf("You: %s  ·  %s", me.User, me.IP))
	}
	refreshYouLabel()

	netLabel := widget.NewLabelWithStyle("Network", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})

	// --- peer checklist (shown after tapping the beez button) ---
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
			label := fmt.Sprintf("%s · %s", peer.User, peer.IP)
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

	beezReasonEntry := widget.NewEntry()
	beezReasonEntry.SetPlaceHolder("Why are you beezing?")
	frequentMessages := []string{
		"help!",
		"wait!",
		"join me!",
		"call me!",
	}
	appendEmoji := func(emoji string) {
		if emoji == "" {
			return
		}
		current := strings.TrimSpace(beezReasonEntry.Text)
		if current == "" {
			beezReasonEntry.SetText(emoji)
			return
		}
		beezReasonEntry.SetText(truncatebeezReason(current + " " + emoji))
	}

	emojiGrid := container.NewGridWithColumns(6)
	for _, emoji := range []rune(beezEmojis) {
		e := string(emoji)
		emojiGrid.Add(newTappableEmoji(e, func() { appendEmoji(e) }))
	}
	quickMsgGrid := container.NewGridWithColumns(2)
	for _, msg := range frequentMessages {
		m := msg
		quickMsgGrid.Add(widget.NewButton(m, func() {
			beezReasonEntry.SetText(truncatebeezReason(m))
		}))
	}

	sendSelected := func() {
		if len(selected) == 0 {
			a.SendNotification(&fyne.Notification{
				Title:   "beez",
				Content: "Select at least one person first.",
			})
			return
		}
		reason := beezReasonEntry.Text
		sent := 0
		for ip := range selected {
			if err := sendbeez(me, selectedLA, ip, reason); err == nil {
				sent++
			}
		}
		a.SendNotification(&fyne.Notification{
			Title:   "beez sent",
			Content: fmt.Sprintf("Pinged %d colleague(s).", sent),
		})
	}
	beezReasonEntry.OnSubmitted = func(string) { sendSelected() }

	beezSelectedBtn := widget.NewButton("beez selected", func() { sendSelected() })
	beezSelectedBtn.Importance = widget.HighImportance

	userPanel := container.NewVBox(
		widget.NewSeparator(),
		widget.NewLabelWithStyle("Pick colleagues", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		container.NewBorder(nil, nil, nil, nil, peerScroll),
		beezReasonEntry,
		quickMsgGrid,
		emojiGrid,
		beezSelectedBtn,
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

	beezBtn := newTappableIcon(icon, beezButtonSize, togglePanel)

	hint := widget.NewLabelWithStyle("Tap bee to pick who to beez", fyne.TextAlignCenter, fyne.TextStyle{Italic: true})

	compact := container.NewVBox(
		title,
		youLabel,
		widget.NewSeparator(),
		netLabel,
		ipSelect,
		container.NewCenter(beezBtn),
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
