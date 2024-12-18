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

.PHONY: build up down test-generate-synth aws-login aws-push deploy-all

# ---------------------------------------
# local dev only
# ---------------------------------------
down:
	docker-compose down --remove-orphans

up:
	docker-compose up

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

deploy-all: build aws-login
	# Tag image for ECR
	docker tag $(IMAGE_NAME) $(ECR_URL)/$(ECR_REPO):latest
	# Push to ECR
	docker push $(ECR_URL)/$(ECR_REPO):latest
	# Deploy infrastructure
	cd infra && npm install && cdktf deploy --auto-approve

# optional prompt
test-generate-synth:
	curl -X POST \
	  http://localhost:8080/generate-synth \
	  -H "Content-Type: application/json" \
	  -d '{"prompt":"","provider":"openai","model":"o1-preview"}'
