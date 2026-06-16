# Simple build helper for beez.
#
#   make tidy    -> fetch/refresh dependencies (run this first)
#   make         -> release build for this machine
#   make run     -> run on your current machine
#   make clean   -> remove build output
#
# App icon source: assets/beez-icon.png (used on Windows and Linux).
# Windows needs TDM-GCC or MinGW on PATH for windres.

BINARY := beez
ICON_PNG := assets/beez-icon.png
ICON_ICO := assets/beez-icon.ico
EMBED_PNG := internal/beez/assets/beez-icon.png
DIST := dist

ifeq ($(OS),Windows_NT)
EXE_EXT := .exe
RM_DIST := rmdir /s /q
MKDIR := if not exist "$(DIST)" mkdir "$(DIST)"
WINDRES := windres
LDFLAGS := -s -w -linkmode=internal -H=windowsgui
else
EXE_EXT :=
RM_DIST := rm -rf
MKDIR := mkdir -p $(DIST)
LDFLAGS := -s -w
endif

OUT := $(DIST)/$(BINARY)$(EXE_EXT)
SYSO := cmd/beez/resource.syso

.PHONY: all tidy run build icon clean

all: build

tidy:
	go mod tidy

run:
	go run ./cmd/beez

$(EMBED_PNG): $(ICON_PNG)
	go run ./tools/syncicon

$(ICON_ICO): $(ICON_PNG)
	go run ./tools/png2ico

ifeq ($(OS),Windows_NT)
icon: resource.rc $(ICON_ICO)
	$(WINDRES) resource.rc -o $(SYSO)

build: $(DIST) $(EMBED_PNG) icon
	set CGO_ENABLED=1&& go build -buildmode=exe -ldflags "$(LDFLAGS)" -o $(OUT) ./cmd/beez
else
build: $(DIST) $(EMBED_PNG)
	go build -ldflags "$(LDFLAGS)" -o $(OUT) ./cmd/beez
endif

$(DIST):
	$(MKDIR)

clean:
ifeq ($(OS),Windows_NT)
	-if exist "$(DIST)" $(RM_DIST) "$(DIST)"
	-if exist "$(SYSO)" del /q "$(SYSO)"
	-if exist "$(ICON_ICO)" del /q "$(ICON_ICO)"
else
	-$(RM_DIST) $(DIST)
	-rm -f $(SYSO) $(ICON_ICO)
endif
