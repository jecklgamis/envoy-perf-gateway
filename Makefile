IMAGE_NAME:=envoy-perf-gateway
IMAGE_TAG:=$(shell git rev-parse --abbrev-ref HEAD 2>/dev/null || echo local)

default:
	@cat ./Makefile
image:
	docker build -t $(IMAGE_NAME):$(IMAGE_TAG) .
# HTTP source mode: config_fetcher.py inside the container polls
# config_server/config_server.py, which you must run first (make -C
# config_server up). host.docker.internal lets the container reach the
# host. Set CONFIG_API_TOKEN if the server requires one.
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
venv:
	python3 -m venv .venv
	.venv/bin/pip install -e "./gatewayctl[s3]"
# gatewayctl is a self-contained sub-project (own pyproject.toml, own
# Makefile, templates bundled as package data) - `pip install
# gatewayctl/dist/*.whl` works standalone, not just editable from this repo.
build-gatewayctl:
	$(MAKE) -C gatewayctl build
all: image
up: all run
