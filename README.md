# beez

A tiny desktop app for the office. When you run it, it opens a small window
showing your own IP address and username, then broadcasts that on the local
network every few seconds. Every other PC on the same network running beez
shows up in the same window, so anyone can see who's online and what their
IP is — no more shouting across the room or pinging on chat to ask "what's
your IP?" when you've got headphones in and missed someone calling you.

It uses the [Fyne](https://fyne.io) GUI toolkit and is now organized with a
`cmd/beez` entrypoint plus reusable package code.

## How it works

- On startup it works out your username (`os/user`, with a fallback to the
  `USER`/`USERNAME` environment variables) and the IP address your machine
  would use to talk to the rest of the LAN.
- It broadcasts `{user, host, ip}` as a small JSON message over UDP on port
  `9876` every 3 seconds, and listens for the same broadcast from everyone
  else.
- Anyone not heard from for 12 seconds quietly drops off the list (they
  probably closed the app).
- This only works for machines on the same local subnet/VLAN — broadcasts
  don't normally cross routers, which is the same reason this can't be
  abused from outside your office network either.

## Prerequisites

Fyne uses a small amount of C code under the hood for graphics, so each
platform needs a C compiler in addition to Go (this is a one-time setup
per machine, not per build):

- **Linux (Debian/Ubuntu):** `sudo apt install gcc libgl1-mesa-dev xorg-dev`
- **Linux (Fedora):** `sudo dnf install gcc libXcursor-devel libXrandr-devel mesa-libGL-devel libXi-devel libXinerama-devel libXxf86vm-devel`
- **Windows:** install a C compiler such as [mingw-w64](https://www.mingw-w64.org/) or [TDM-GCC](https://jmeubank.github.io/tdm-gcc/) and make sure it's on your `PATH`
- Go 1.21 or newer on both

## Building

First, on either platform, fetch the dependency (only needed once, or
whenever you update Fyne):

```
go mod tidy
```

### The easy way: build natively on each OS

This is the simplest, most reliable option and needs no special tooling
beyond what's listed above:

```
# on Linux
go build -o beez ./cmd/beez

# on Windows (hides the console window)
go build -ldflags -H=windowsgui -o beez.exe ./cmd/beez
```

Or with the included Makefile: `make linux` / `make windows`.

### Cross-compiling from one machine (e.g. build the Windows .exe from Linux)

Because of the C-code dependency above, plain `go build` with `GOOS=windows`
set from a Linux machine usually won't link successfully unless you have a
full mingw-w64 cross toolchain configured. The easier route is
[fyne-cross](https://github.com/fyne-io/fyne-cross), a wrapper from the
Fyne team that runs the build inside a Docker image that already has
everything set up:

```
go install github.com/fyne-io/fyne-cross@latest
docker --version   # make sure Docker is installed and running

fyne-cross windows -arch=amd64 .   # produces a .exe under fyne-cross/dist/windows-amd64
fyne-cross linux   -arch=amd64 .   # produces a binary under fyne-cross/dist/linux-amd64
```

Or with the Makefile: `make cross-windows` / `make cross-linux`.

## Running

Just run the binary — no flags or config needed:

```
./beez          # Linux
beez.exe        # Windows
```

The first time it runs, Windows Firewall (or a Linux firewall like ufw)
may ask to allow the app to send/receive on the network — say yes, or
nobody else will show up in the list.

## Notes / things to tweak

- `appPort` in `internal/beez/main.go` (default `9876`) — change it if that port is
  already used by something else on your network.
- `announceEvery` / `peerTimeout` — how chatty the broadcasts are and how
  quickly someone disappears after closing the app.
- If a machine has more than one active network connection (e.g. a VPN),
  pick your **office LAN** address in the Network dropdown — not the one
  marked `· VPN`. With VPN on, the app defaults to LAN when it can find one.
  Broadcast discovery only reaches people on the same subnet; beez pings are
  sent directly to their IP.
- **Windows can't see you but you can see them?** Make sure both sides picked
  the same kind of network (LAN, not VPN) and that Windows Firewall allows
  inbound UDP on port `9876` for `beez`.
