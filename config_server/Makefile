IMAGE_NAME:=envoy-perf-gateway-config-server
IMAGE_TAG:=$(shell git rev-parse --abbrev-ref HEAD 2>/dev/null || echo local)

default:
	@cat ./Makefile
image:
	docker build -t $(IMAGE_NAME):$(IMAGE_TAG) .
# storage/ persists uploaded configs across container restarts.
# Set API_TOKEN in your shell to require it on every /config/* request.
run:
	mkdir -p storage
	-docker rm -f $(IMAGE_NAME) 2>/dev/null
	docker run --name $(IMAGE_NAME) \
		-p 8090:8090 \
		-v $(PWD)/storage:/data \
		-e API_TOKEN \
		$(IMAGE_NAME):$(IMAGE_TAG)
stop:
	docker stop $(IMAGE_NAME) && docker rm $(IMAGE_NAME)
logs:
	docker logs -f $(IMAGE_NAME)
all: image
up: all run
