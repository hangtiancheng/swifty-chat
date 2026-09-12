.PHONY: dev build

dev: ## Hot-reload the chat server with air
	air -c .air.toml

build: ## Build the chat server into ./tmp
	go build -o ./tmp/main ./cmd
