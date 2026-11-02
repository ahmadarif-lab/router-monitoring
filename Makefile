VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION)
BIN := router-monitoring

.PHONY: build test run demo dist clean install
build:
	go build -trimpath -ldflags '$(LDFLAGS)' -o $(BIN) ./cmd/router-monitoring

test:
	go vet ./... && go test -race ./...

run: build
	./$(BIN)

demo: build
	./$(BIN) -demo

install: build
	install -m 0755 $(BIN) $(HOME)/.local/bin/$(BIN)

# Release binaries for every platform in dist/
dist:
	@mkdir -p dist
	@for t in darwin/arm64 darwin/amd64 linux/amd64 linux/arm64 windows/amd64; do \
	  os=$${t%/*}; arch=$${t#*/}; ext=; [ $$os = windows ] && ext=.exe; \
	  echo "build $$os/$$arch"; \
	  CGO_ENABLED=0 GOOS=$$os GOARCH=$$arch go build -trimpath -ldflags '$(LDFLAGS)' -o dist/$(BIN)-$$os-$$arch$$ext ./cmd/router-monitoring || exit 1; \
	done

clean:
	rm -rf dist $(BIN)
