VERSION := $(shell tr -d '\r\n' < VERSION)
IMAGE := muni:v$(VERSION)

.PHONY: test build web docker offline hooks

test:
	scripts/check-webui-placeholder.sh
	go test ./...
	cd frontend && npm run lint

hooks:
	git config core.hooksPath scripts/git-hooks
	@echo "git hooks enabled from scripts/git-hooks"

web:
	cd frontend && npm ci && npm run build

build: web
	go build -trimpath -ldflags="-X main.version=v$(VERSION)" -o muni ./cmd/muni

docker:
	docker build --build-arg VERSION=v$(VERSION) -t $(IMAGE) .

offline:
	./scripts/package-offline.sh v$(VERSION)
