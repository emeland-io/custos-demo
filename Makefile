.PHONY: frontend-dist test build docker-build

frontend-dist:
	cd frontend && npm ci && npm run build
	find internal/webassets/dist -mindepth 1 ! -name README.txt -delete
	cp -r frontend/dist/. internal/webassets/dist/
	git checkout -- internal/webassets/dist/README.txt

test:
	go test ./...
	cd frontend && npm run test

build: frontend-dist
	go build -o bin/notebook-server ./cmd/server

docker-build:
	docker build -t notebook:dev .
