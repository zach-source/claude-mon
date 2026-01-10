.PHONY: build install clean test run

BINARY_NAME=claude-follow-tui
BUILD_DIR=./bin

build:
	go build -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/claude-follow-tui

install: build
	cp $(BUILD_DIR)/$(BINARY_NAME) ~/go/bin/$(BINARY_NAME)

clean:
	rm -rf $(BUILD_DIR)
	go clean

test:
	go test ./...

run: build
	$(BUILD_DIR)/$(BINARY_NAME)

# Development: rebuild and run
dev:
	go run ./cmd/claude-follow-tui

# Send test message to running TUI
test-send:
	echo '{"tool_name":"Edit","tool_input":{"file_path":"test.go","old_string":"old","new_string":"new"}}' | go run ./cmd/claude-follow-tui send
