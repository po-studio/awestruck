# Docker Configuration
PLATFORM ?= linux/arm64
IMAGE_NAME = server-$(subst /,-,$(PLATFORM))
CONTAINER_NAME = $(IMAGE_NAME)-instance
BROWSER_PORT = 8080

# STUN server configuration
STUN_IMAGE_NAME = stun-server-$(subst /,-,$(PLATFORM))

# AWS Configuration
AWS_REGION ?= us-east-1
AWS_ACCOUNT_ID ?= $(shell aws sts get-caller-identity --query Account --output text)
ECR_REPO = po-studio/awestruck
STUN_ECR_REPO = po-studio/stun-server
ECR_URL = $(AWS_ACCOUNT_ID).dkr.ecr.$(AWS_REGION).amazonaws.com

.PHONY: build build-stun up down test-generate-synth aws-login aws-push aws-push-stun deploy-all

# ---------------------------------------
# local dev only
# ---------------------------------------
down:
	docker-compose down --remove-orphans

up:
	docker-compose up

r: down upb

upb:
	docker compose up --build
# ---------------------------------------

# ---------------------------------------
# deployment build, push, and deploy
# ---------------------------------------
build:
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

build-stun:
	@if ! docker buildx inspect mybuilder > /dev/null 2>&1; then \
		docker buildx create --use --name mybuilder; \
	else \
		docker buildx use mybuilder; \
	fi
	docker buildx inspect --bootstrap
	DOCKER_BUILDKIT=1 docker buildx build \
		--platform $(PLATFORM) \
		--cache-from type=local,src=/tmp/.buildx-cache-stun \
		--cache-to type=local,dest=/tmp/.buildx-cache-stun-new \
		-t $(STUN_IMAGE_NAME) \
		-f Dockerfile.stun \
		--load .
	# Move the new cache to replace the old cache
	rm -rf /tmp/.buildx-cache-stun
	mv /tmp/.buildx-cache-stun-new /tmp/.buildx-cache-stun

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

aws-push-stun: aws-login
	# Pull the latest image from ECR to use as cache
	docker pull $(ECR_URL)/$(STUN_ECR_REPO):latest || true
	
	# Build with cache from both local and ECR
	DOCKER_BUILDKIT=1 docker buildx build \
		--platform $(PLATFORM) \
		--cache-from type=local,src=/tmp/.buildx-cache-stun \
		--cache-from $(ECR_URL)/$(STUN_ECR_REPO):latest \
		--cache-to type=local,dest=/tmp/.buildx-cache-stun-new \
		-t $(STUN_IMAGE_NAME):latest \
		-t $(ECR_URL)/$(STUN_ECR_REPO):latest \
		-f Dockerfile.stun \
		--load .
	
	# Push to ECR with all layers
	docker push $(ECR_URL)/$(STUN_ECR_REPO):latest
	
	# Move the new cache
	rm -rf /tmp/.buildx-cache-stun
	mv /tmp/.buildx-cache-stun-new /tmp/.buildx-cache-stun

deploy-all: build build-stun aws-login
	# Tag WebRTC server image for ECR
	docker tag $(IMAGE_NAME) $(ECR_URL)/$(ECR_REPO):latest
	# Push WebRTC server to ECR
	docker push $(ECR_URL)/$(ECR_REPO):latest
	
	# Tag STUN server image for ECR
	docker tag $(STUN_IMAGE_NAME) $(ECR_URL)/$(STUN_ECR_REPO):latest
	# Push STUN server to ECR
	docker push $(ECR_URL)/$(STUN_ECR_REPO):latest
	
	# Deploy infrastructure
	cd infra && npm install && cdktf deploy --auto-approve

test-generate-synth:
	curl -X POST \
		http://localhost:8080/generate-synth \
		-H "Content-Type: application/json" \
		-H "Awestruck-API-Key: $${AWESTRUCK_API_KEY}" \
		-d '{"prompt":"","provider":"openai","model":"o1-preview"}'