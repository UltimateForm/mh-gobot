BINARY=mh-gobot
IMAGE=mh-gobot
ENV_FILE=.env

.PHONY: test build run watch-cons docker-build docker-run docker-run-detached docker-kill-detached

test:
	go test ./...

share-db:
	podman unshare chown -R $(id -u):$(id -g) ~/.mh-gobot

unshare-db:
	podman unshare chown -R $(podman unshare id -u):$(podman unshare id -g) ~/.mh-gobot



build:
	go build -o .out/$(BINARY) .

run: build
	.out/$(BINARY)

watch-cons:
	watch -n 1 'ss -tp | grep $(BINARY)'

docker-build:
	docker build -t $(IMAGE) .

docker-run:
	docker run --rm --env-file $(ENV_FILE) -v $(HOME)/.mh-gobot:/home/appuser/.mh-gobot:Z $(IMAGE)

docker-run-detached: docker-kill-detached
	docker run -d --name $(IMAGE) --env-file $(ENV_FILE) -v $(HOME)/.mh-gobot:/home/appuser/.mh-gobot:Z $(IMAGE)

docker-kill-detached:
	docker rm -f $(IMAGE) 2>/dev/null || true
