# Simple build helper for bizz.
#
#   make tidy              -> fetch/refresh dependencies (run this first)
#   make run                -> run on your current machine
#   make linux               -> native build with embedded app icon (needs fyne CLI)
#   make windows              -> native build with embedded app icon (needs fyne CLI)
#   make linux-plain          -> plain go build, no embedded binary icon
#   make windows-plain        -> plain go build, no embedded binary icon
#   make cross-windows         -> cross-compile a Windows .exe (needs Docker)
#   make cross-linux           -> cross-compile a Linux binary (needs Docker)
#   make clean                  -> remove build output
#
# For embedded taskbar / file icons install the Fyne tool once:
#   go install fyne.io/fyne/v2/cmd/fyne@latest

BINARY := bizz
ICON := assets/bizz-icon.svg
DIST := dist

.PHONY: tidy run linux windows linux-plain windows-plain cross-windows cross-linux clean

tidy:
	go mod tidy

run:
	go run .

linux: $(DIST)
	fyne package -os linux -name $(BINARY) -icon $(ICON) -release -executable $(DIST)/$(BINARY)-linux-amd64 .

windows: $(DIST)
	fyne package -os windows -name $(BINARY) -icon $(ICON) -release -executable $(DIST)/$(BINARY)-windows-amd64.exe .

linux-plain: $(DIST)
	GOOS=linux GOARCH=amd64 go build -o $(DIST)/$(BINARY)-linux-amd64 .

windows-plain: $(DIST)
	GOOS=windows GOARCH=amd64 go build -ldflags -H=windowsgui -o $(DIST)/$(BINARY)-windows-amd64.exe .

# These two need Docker and the fyne-cross tool:
#   go install github.com/fyne-io/fyne-cross@latest
cross-windows:
	fyne-cross windows -arch=amd64 -icon $(ICON) .

cross-linux:
	fyne-cross linux -arch=amd64 -icon $(ICON) .

$(DIST):
	mkdir -p $(DIST)

clean:
	rm -rf $(DIST) fyne-cross
