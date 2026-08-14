APP    := wsf
VERSION := 0.1.0
LDFLAGS := -s -w -X main.version=$(VERSION)

.PHONY: build vet test release clean

build:
	go build -trimpath -ldflags "$(LDFLAGS)" -o bin/wsf .

vet:
	go vet ./...

test: vet
	go test ./...

release:
	@mkdir -p dist
	GOOS=windows GOARCH=amd64 go build -trimpath -ldflags "$(LDFLAGS)" -o dist/wsf-windows-amd64.exe .
	GOOS=windows GOARCH=arm64 go build -trimpath -ldflags "$(LDFLAGS)" -o dist/wsf-windows-arm64.exe .
	GOOS=linux GOARCH=amd64 go build -trimpath -ldflags "$(LDFLAGS)" -o dist/wsf-linux-amd64 .
	GOOS=linux GOARCH=arm64 go build -trimpath -ldflags "$(LDFLAGS)" -o dist/wsf-linux-arm64 .
	@if command -v sha256sum >/dev/null 2>&1; then sha256sum dist/* > dist/sha256sums.txt; fi

clean:
	rm -rf bin dist
