.PHONY: test
.POSIX:
.SUFFIXES:

GIT_COMMIT := $(shell git rev-parse HEAD 2> /dev/null)
GIT_TAG := $(shell git describe --abbrev=0 --tags)

SERVICE = syncyomi
GO = go
RM = rm
GOFLAGS = "-X main.commit=$(GIT_COMMIT) -X main.version=$(GIT_TAG)"
PREFIX = /usr/local
BINDIR = bin

all: clean build

deps:
	sudo npm install -g pnpm
	cd web && pnpm install
	go mod download

build: deps build/web build/app

build/app:
	go build -ldflags $(GOFLAGS) -o bin/$(SERVICE) main.go

build/web:
	cd web && pnpm build

clean:
	$(RM) -rf bin

install: all
	echo $(DESTDIR)$(PREFIX)/$(BINDIR)
	mkdir -p $(DESTDIR)$(PREFIX)/$(BINDIR)
	cp -f bin/$(SERVICE) $(DESTDIR)$(PREFIX)/$(BINDIR)