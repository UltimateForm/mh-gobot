BINARY=ryard
IMAGE=ryard

.PHONY: test build run docker-build docker-run

test:
	go test ./...

build:
	go build -o .out/$(BINARY) .

run: build
	.out/$(BINARY)

docker-build:
	docker build -t $(IMAGE) .

docker-run:
	docker run --rm --env-file .env -v $(HOME)/.ryard:/root/.ryard $(IMAGE)

docker-run-detached:
	docker run -d --name $(IMAGE) --env-file .env -v $(HOME)/.ryard:/root/.ryard $(IMAGE)

docker-kill-detached:
	docker rm -f $(IMAGE)
