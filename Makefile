IMAGE_NAME:=envoy-perf-gateway
IMAGE_TAG:=$(shell git rev-parse --abbrev-ref HEAD 2>/dev/null || echo local)

default:
	@cat ./Makefile
image:
	docker build -t $(IMAGE_NAME):$(IMAGE_TAG) .
# HTTP source mode: the config-fetcher binary inside the container polls
# config_server, which you must run first (make -C config_server up).
# host.docker.internal lets the container reach the host. Set
# CONFIG_API_TOKEN if the server requires one.
run:
	-docker rm -f $(IMAGE_NAME) 2>/dev/null
	docker run --name $(IMAGE_NAME) \
		-p 8080:8080 -p 9901:9901 \
		-e CONFIG_SOURCE_KIND=http \
		-e CONFIG_SOURCE_URL=http://host.docker.internal:8090 \
		-e CONFIG_API_TOKEN \
		$(IMAGE_NAME):$(IMAGE_TAG)
# S3 source mode: run `gatewayctl push-s3 --bucket ...` after add-backend
# instead of relying on the local HTTP server. Needs AWS credentials in the
# container's environment (mount ~/.aws, or pass -e AWS_ACCESS_KEY_ID etc).
run-s3:
	docker run -d --name $(IMAGE_NAME) \
		-p 8080:8080 -p 9901:9901 \
		-e CONFIG_SOURCE_KIND=s3 \
		-e CONFIG_S3_BUCKET=$(CONFIG_S3_BUCKET) \
		-e CONFIG_S3_PREFIX=$(CONFIG_S3_PREFIX) \
		-e AWS_ACCESS_KEY_ID -e AWS_SECRET_ACCESS_KEY -e AWS_SESSION_TOKEN -e AWS_REGION \
		$(IMAGE_NAME):$(IMAGE_TAG)
stop:
	docker stop $(IMAGE_NAME) && docker rm $(IMAGE_NAME)
run-shell:
	docker run -i -t $(IMAGE_NAME):$(IMAGE_TAG) /bin/bash
exec-shell:
	docker exec -it `docker ps | grep $(IMAGE_NAME) | awk '{print $$1}'` /bin/bash
# gatewayctl is a self-contained Go module (own go.mod, own Makefile) -
# a plain static binary, installable anywhere with no runtime dependency.
build-gatewayctl:
	$(MAKE) -C gatewayctl build
# Cross-compiles gatewayctl for macOS (arm64/amd64) and Linux (amd64/arm64)
# into gatewayctl/dist/ - hand these to teammates directly, no Go toolchain
# needed on their end.
build-gatewayctl-all:
	$(MAKE) -C gatewayctl build-all
all: image
up: all run
