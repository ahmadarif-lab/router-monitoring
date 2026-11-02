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

# Release archives for every platform in dist/: <name>_<version>_<os>_<arch>.tar.gz (zip on Windows),
# each holding the binary, LICENSE and README.md.
dist:
	@rm -rf dist && mkdir -p dist
	@for t in darwin/arm64 darwin/amd64 linux/amd64 linux/arm64 windows/amd64; do \
	  os=$${t%/*}; arch=$${t#*/}; ext=; [ $$os = windows ] && ext=.exe; \
	  name=$(BIN)_$(VERSION)_$${os}_$${arch}; dir=dist/$$name; \
	  echo "build $$os/$$arch"; mkdir -p $$dir; \
	  CGO_ENABLED=0 GOOS=$$os GOARCH=$$arch go build -trimpath -ldflags '$(LDFLAGS)' -o $$dir/$(BIN)$$ext ./cmd/router-monitoring || exit 1; \
	  cp LICENSE README.md $$dir/; \
	  if [ $$os = windows ]; then (cd dist && zip -q -r $$name.zip $$name); else tar -C dist -czf dist/$$name.tar.gz $$name; fi; \
	  rm -rf $$dir; \
	done

clean:
	rm -rf dist $(BIN)
