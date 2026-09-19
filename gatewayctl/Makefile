BINARY := gatewayctl
DIST := dist

.DEFAULT_GOAL := build

# Builds a binary for your current OS/arch.
build:
	go build -o $(BINARY) .
# Cross-compiles for macOS (arm64/amd64) and Linux (amd64/arm64) into dist/.
build-all: clean
	mkdir -p $(DIST)
	GOOS=darwin  GOARCH=arm64 go build -o $(DIST)/$(BINARY)-darwin-arm64 .
	GOOS=darwin  GOARCH=amd64 go build -o $(DIST)/$(BINARY)-darwin-amd64 .
	GOOS=linux   GOARCH=amd64 go build -o $(DIST)/$(BINARY)-linux-amd64 .
	GOOS=linux   GOARCH=arm64 go build -o $(DIST)/$(BINARY)-linux-arm64 .
install: build
	go install .
test:
	go test ./...
clean:
	rm -rf $(BINARY) $(DIST)
