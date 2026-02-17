BINARY := smart-monitor
BUILD_DIR := bin
GOFLAGS := -trimpath -ldflags="-s -w"

.PHONY: build test lint clean install

build:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build $(GOFLAGS) -o $(BUILD_DIR)/$(BINARY) ./cmd/smart-monitor

test:
	go test -v -race ./...

lint:
	go vet ./...

clean:
	rm -rf $(BUILD_DIR)

install: build
	install -d /usr/local/bin
	install -m 755 $(BUILD_DIR)/$(BINARY) /usr/local/bin/$(BINARY)
	install -d /etc/synology-monitor
	test -f /etc/synology-monitor/config.yaml || install -m 600 config.yaml.example /etc/synology-monitor/config.yaml
	install -m 644 synology-monitor.service /etc/systemd/system/synology-monitor.service
	systemctl daemon-reload
