.PHONY: build test install
PREFIX ?= $(HOME)/.local
build:
	cd web && npm ci && npm run build
	go build -o bin/maced ./cmd/maced
	go build -o bin/mace ./cmd/mace
test:
	go test ./...
	cd web && npm run build
install: build
	install -d $(PREFIX)/bin
	install -m 755 bin/mace bin/maced $(PREFIX)/bin/
	install -d $(PREFIX)/share/mace/web
	cp -R web/dist/. $(PREFIX)/share/mace/web/
