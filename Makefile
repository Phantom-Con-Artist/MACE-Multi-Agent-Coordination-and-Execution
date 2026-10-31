# MACE monorepo: server/ (Go daemon and CLI), web/ (desktop app),
# android/ (MACE for Android). One VERSION covers all three.
.PHONY: build server web android test install dist clean

PREFIX ?= $(HOME)/.local
VERSION := $(shell cat VERSION)
LDFLAGS := -s -w -X mace/internal/bootstrap.Version=$(VERSION)
GOOS ?= $(shell go env GOOS)
GOARCH ?= $(shell go env GOARCH)

build: web server

web:
	cd web && npm ci && npm run build

server:
	cd server && go build -ldflags "$(LDFLAGS)" -o ../bin/maced ./cmd/maced
	cd server && go build -ldflags "$(LDFLAGS)" -o ../bin/mace ./cmd/mace

android:
	cd android && ./gradlew assembleDebug

test:
	cd server && go test ./...
	cd web && npm run build

install: build
	install -d $(PREFIX)/bin
	install -m 755 bin/mace bin/maced $(PREFIX)/bin/
	ln -sf mace $(PREFIX)/bin/mace-server
	install -d $(PREFIX)/share/mace/web
	cp -R web/dist/. $(PREFIX)/share/mace/web/

# dist packages the hub for one platform: dist/mace-<version>-<os>-<arch>.tar.gz
# holds bin/mace, bin/maced, bin/mace-server, and share/mace/web, ready to
# unpack into ~/.local or /usr/local. Build the web app first (make web).
dist:
	rm -rf build/pkg && mkdir -p build/pkg/bin build/pkg/share/mace dist
	cd server && CGO_ENABLED=0 GOOS=$(GOOS) GOARCH=$(GOARCH) go build -trimpath -ldflags "$(LDFLAGS)" -o ../build/pkg/bin/maced ./cmd/maced
	cd server && CGO_ENABLED=0 GOOS=$(GOOS) GOARCH=$(GOARCH) go build -trimpath -ldflags "$(LDFLAGS)" -o ../build/pkg/bin/mace ./cmd/mace
	ln -sf mace build/pkg/bin/mace-server
	cp -R web/dist build/pkg/share/mace/web
	cp README.md VERSION build/pkg/
	tar -C build/pkg -czf dist/mace-$(VERSION)-$(GOOS)-$(GOARCH).tar.gz .

clean:
	rm -rf bin build dist web/dist
