VERSION := $(shell tr -d '\r\n' < VERSION)
IMAGE := muni:v$(VERSION)

.PHONY: test build web docker offline

test:
	go test ./...
	cd frontend && npm run lint

web:
	cd frontend && npm ci && npm run build

build: web
	go build -trimpath -ldflags="-X main.version=v$(VERSION)" -o muni ./cmd/muni

docker:
	docker build --build-arg VERSION=v$(VERSION) -t $(IMAGE) .

offline:
	./scripts/package-offline.sh v$(VERSION)
