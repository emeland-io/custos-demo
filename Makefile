.PHONY: frontend-dist test build docker-build

frontend-dist:
	cd frontend && npm ci && npm run build
	rm -rf internal/webassets/dist
	mkdir -p internal/webassets/dist
	cp -r frontend/dist/. internal/webassets/dist/

test:
	go test ./...
	cd frontend && npm run test

build: frontend-dist
	go build -o bin/notebook-server ./cmd/server

docker-build:
	docker build -t notebook:dev .
