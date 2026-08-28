.PHONY: dev swiftx build

dev: ## Hot-reload the chat server with air
	air -c .air.toml

swiftx: ## Run the swiftx TUI
	go run ./cmd/swiftx

build: ## Build both binaries into ./tmp
	go build -o ./tmp/main ./cmd
	go build -o ./tmp/swiftx ./cmd/swiftx

sync:
	rm -rf ~/.swiftx
	cp -r ~/.swifty ~/.swiftx
