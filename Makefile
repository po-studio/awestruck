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
ECR_WEBRTC_REPO = po-studio/awestruck/services/webrtc
ECR_STUN_REPO = po-studio/awestruck/services/stun
ECR_WEBRTC_URL = $(AWS_ACCOUNT_ID).dkr.ecr.$(AWS_REGION).amazonaws.com/$(ECR_WEBRTC_REPO)
ECR_STUN_URL = $(AWS_ACCOUNT_ID).dkr.ecr.$(AWS_REGION).amazonaws.com/$(ECR_STUN_REPO)

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
		docker login --username AWS --password-stdin $(AWS_ACCOUNT_ID).dkr.ecr.$(AWS_REGION).amazonaws.com
	# Create ECR repositories if they don't exist
	aws ecr describe-repositories --repository-names $(ECR_WEBRTC_REPO) || \
		aws ecr create-repository --repository-name $(ECR_WEBRTC_REPO)
	aws ecr describe-repositories --repository-names $(ECR_STUN_REPO) || \
		aws ecr create-repository --repository-name $(ECR_STUN_REPO)

aws-push: aws-login
	# Pull the latest image from ECR to use as cache
	docker pull $(ECR_WEBRTC_URL):latest || true
	
	# Build with cache from both local and ECR
	DOCKER_BUILDKIT=1 docker buildx build \
		--platform $(PLATFORM) \
		--cache-from type=local,src=/tmp/.buildx-cache \
		--cache-from $(ECR_WEBRTC_URL):latest \
		--cache-to type=local,dest=/tmp/.buildx-cache-new \
		-t $(IMAGE_NAME):latest \
		-t $(ECR_WEBRTC_URL):latest \
		--load .
	
	# Push to ECR with all layers
	docker push $(ECR_WEBRTC_URL):latest
	
	# Move the new cache
	rm -rf /tmp/.buildx-cache
	mv /tmp/.buildx-cache-new /tmp/.buildx-cache

aws-push-stun: aws-login
	# Pull the latest image from ECR to use as cache
	docker pull $(ECR_STUN_URL):latest || true
	
	# Build with cache from both local and ECR
	DOCKER_BUILDKIT=1 docker buildx build \
		--platform $(PLATFORM) \
		--cache-from type=local,src=/tmp/.buildx-cache-stun \
		--cache-from $(ECR_STUN_URL):latest \
		--cache-to type=local,dest=/tmp/.buildx-cache-stun-new \
		-t $(STUN_IMAGE_NAME):latest \
		-t $(ECR_STUN_URL):latest \
		-f Dockerfile.stun \
		--load .
	
	# Push to ECR with all layers
	docker push $(ECR_STUN_URL):latest
	
	# Move the new cache
	rm -rf /tmp/.buildx-cache-stun
	mv /tmp/.buildx-cache-stun-new /tmp/.buildx-cache-stun

deploy-all: build build-stun aws-login
	# Tag WebRTC server image for ECR
	docker tag $(IMAGE_NAME) $(ECR_WEBRTC_URL):latest
	# Push WebRTC server to ECR
	docker push $(ECR_WEBRTC_URL):latest
	
	# Tag STUN server image for ECR
	docker tag $(STUN_IMAGE_NAME) $(ECR_STUN_URL):latest
	# Push STUN server to ECR
	docker push $(ECR_STUN_URL):latest
	
	# Deploy infrastructure
	cd infra && npm install && cdktf deploy --auto-approve

test-generate-synth:
	curl -X POST \
		http://localhost:8080/generate-synth \
		-H "Content-Type: application/json" \
		-H "Awestruck-API-Key: $${AWESTRUCK_API_KEY}" \
		-d '{"prompt":"","provider":"openai","model":"o1-preview"}'