# Simple build helper for beez.
#
#   make tidy    -> fetch/refresh dependencies (run this first)
#   make         -> release build for this machine
#   make run     -> run on your current machine
#   make clean   -> remove build output
#
# Windows needs TDM-GCC or MinGW on PATH (same as ge-modbus-browser).

BINARY := beez
ICON_SVG := assets/beez-icon.svg
ICON_PNG := assets/beez-icon.png
ICON_ICO := assets/beez-icon.ico
DIST := dist

ifeq ($(OS),Windows_NT)
	EXE_EXT := .exe
	RM := rmdir /s /q
	MKDIR := if not exist "$(DIST)" mkdir "$(DIST)"
	WINDRES := windres
	LDFLAGS := -s -w -linkmode=internal -H=windowsgui
else
	EXE_EXT :=
	RM := rm -rf
	MKDIR := mkdir -p $(DIST)
	LDFLAGS := -s -w
endif

OUT := $(DIST)/$(BINARY)$(EXE_EXT)

.PHONY: all tidy run build icon clean

all: build

tidy:
	go mod tidy

run:
	go run ./cmd/beez

ifeq ($(OS),Windows_NT)
build: $(DIST) $(ICON_ICO) icon
	set CGO_ENABLED=1&& go build -buildmode=exe -ldflags "$(LDFLAGS)" -o $(OUT) ./cmd/beez

icon: resource.rc $(ICON_ICO)
	$(WINDRES) resource.rc -o resource.syso
else
build: $(DIST)
	go build -ldflags "$(LDFLAGS)" -o $(OUT) ./cmd/beez
endif

$(ICON_PNG) $(ICON_ICO): $(ICON_SVG)
	go run ./tools/genicon

$(DIST):
	$(MKDIR)

clean:
	-$(RM) $(DIST)
	-$(RM) resource.syso
