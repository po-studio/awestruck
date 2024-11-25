# Docker Configuration
PLATFORM ?= linux/arm64
IMAGE_NAME = go-webrtc-server-$(subst /,-,$(PLATFORM))
CONTAINER_NAME = $(IMAGE_NAME)-instance
BROWSER_PORT = 8080

# AWS Configuration
AWS_REGION ?= us-east-1
AWS_ACCOUNT_ID ?= $(shell aws sts get-caller-identity --query Account --output text)
ECR_REPO = po-studio/awestruck
ECR_URL = $(AWS_ACCOUNT_ID).dkr.ecr.$(AWS_REGION).amazonaws.com

# TURN Server Configuration
TURN_CONTAINER = coturn
TURN_NETWORK = awestruck_network

.PHONY: build network up-turn down-turn up down rshell shell aws-login aws-push

network:
	docker network create --driver bridge $(TURN_NETWORK) || true

build: network
	@if ! docker buildx inspect mybuilder > /dev/null 2>&1; then \
		docker buildx create --use --name mybuilder; \
	else \
		docker buildx use mybuilder; \
	fi
	docker buildx inspect --bootstrap
	DOCKER_BUILDKIT=1 docker buildx build \
		--platform $(PLATFORM) \
		--cache-from type=local,src=/tmp/.buildx-cache \
		--cache-to type=local,dest=/tmp/.buildx-cache-new \
		-t $(IMAGE_NAME) \
		--load .
	# Move the new cache to replace the old cache
	rm -rf /tmp/.buildx-cache
	mv /tmp/.buildx-cache-new /tmp/.buildx-cache

up-turn: network
	docker run -d --rm --name $(TURN_CONTAINER) \
		--network $(TURN_NETWORK) \
		-v $(PWD)/coturn.conf:/etc/coturn/turnserver.conf \
		-p 3478:3478 \
		-p 3478:3478/udp \
		-p 5349:5349 \
		-p 5349:5349/udp \
		-p 49152-65535:49152-65535/udp \
		coturn/coturn

down-turn:
	docker stop $(TURN_CONTAINER) || true
	docker rm $(TURN_CONTAINER) || true

up: network
	docker run --rm --name $(CONTAINER_NAME) \
		--platform $(PLATFORM) \
		-p $(BROWSER_PORT):$(BROWSER_PORT) \
		-p 10000-10010:10000-10010/udp \
		--network $(TURN_NETWORK) \
		--shm-size=1g \
		--ulimit memlock=-1 \
		--ulimit stack=67108864 \
		-e JACK_NO_AUDIO_RESERVATION=1 \
		-e JACK_OPTIONS="-R -d dummy" \
		-e JACK_SAMPLE_RATE=48000 \
		$(IMAGE_NAME)

down:
	docker stop $(CONTAINER_NAME) || true
	docker rm $(CONTAINER_NAME) || true

rshell:
	docker run --rm -it $(IMAGE_NAME) /bin/sh

shell:
	docker exec -it $(CONTAINER_NAME) /bin/sh

aws-login:
	aws ecr get-login-password --region $(AWS_REGION) | \
	docker login --username AWS --password-stdin $(ECR_URL)

aws-push: aws-login
	# Pull the latest image from ECR to use as cache
	docker pull $(ECR_URL)/$(ECR_REPO):latest || true
	
	# Build with cache from both local and ECR
	DOCKER_BUILDKIT=1 docker buildx build \
		--platform $(PLATFORM) \
		--cache-from type=local,src=/tmp/.buildx-cache \
		--cache-from $(ECR_URL)/$(ECR_REPO):latest \
		--cache-to type=local,dest=/tmp/.buildx-cache-new \
		-t $(IMAGE_NAME):latest \
		-t $(ECR_URL)/$(ECR_REPO):latest \
		--load .
	
	# Push to ECR with all layers
	docker push $(ECR_URL)/$(ECR_REPO):latest
	
	# Move the new cache
	rm -rf /tmp/.buildx-cache
	mv /tmp/.buildx-cache-new /tmp/.buildx-cache

.PHONY: deploy-all

deploy-all: build aws-login
	# Tag image for ECR
	docker tag $(IMAGE_NAME) $(ECR_URL)/$(ECR_REPO):latest
	# Push to ECR
	docker push $(ECR_URL)/$(ECR_REPO):latest
	# Deploy infrastructure
	cd infra && npm install && cdktf deploy --auto-approve