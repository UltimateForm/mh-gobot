BINARY=mh-gobot
IMAGE=mh-gobot
ENV_FILE=.env

.PHONY: test build run docker-build docker-run docker-run-detached docker-kill-detached

test:
	go test ./...

build:
	go build -o .out/$(BINARY) .

run: build
	.out/$(BINARY)

docker-build:
	docker build -t $(IMAGE) .

docker-run:
	docker run --rm --env-file $(ENV_FILE) -v $(HOME)/.mh-gobot:/root/.mh-gobot $(IMAGE)

docker-run-detached: docker-kill-detached
	docker run -d --name $(IMAGE) --env-file $(ENV_FILE) -v $(HOME)/.mh-gobot:/root/.mh-gobot $(IMAGE)

docker-kill-detached:
	docker rm -f $(IMAGE) 2>/dev/null || true
