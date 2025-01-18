# Docker Configuration
PLATFORM ?= linux/arm64
IMAGE_NAME = server-$(subst /,-,$(PLATFORM))
CONTAINER_NAME = $(IMAGE_NAME)-instance
BROWSER_PORT = 8080

# TURN server configuration
TURN_IMAGE_NAME = turn-server-$(subst /,-,$(PLATFORM))

# AWS Configuration
AWS_REGION ?= us-east-1
AWS_ACCOUNT_ID ?= $(shell aws sts get-caller-identity --query Account --output text)
ECR_WEBRTC_REPO = po-studio/awestruck/services/webrtc
ECR_STUN_REPO = po-studio/awestruck/services/stun
ECR_WEBRTC_URL = $(AWS_ACCOUNT_ID).dkr.ecr.$(AWS_REGION).amazonaws.com/$(ECR_WEBRTC_REPO)
ECR_STUN_URL = $(AWS_ACCOUNT_ID).dkr.ecr.$(AWS_REGION).amazonaws.com/$(ECR_STUN_REPO)
ECR_TURN_REPO = po-studio/awestruck/services/turn
ECR_TURN_URL = $(AWS_ACCOUNT_ID).dkr.ecr.$(AWS_REGION).amazonaws.com/$(ECR_TURN_REPO)

.PHONY: build build-turn up down test-generate-synth aws-login aws-push aws-push-turn deploy-all

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

build-turn:
	@echo "Building TURN server..."
	@mkdir -p /tmp/.buildx-cache-turn
	@docker buildx build \
		--platform $(PLATFORM) \
		--cache-from type=local,src=/tmp/.buildx-cache-turn \
		--cache-to type=local,dest=/tmp/.buildx-cache-turn-new \
		-t $(TURN_IMAGE_NAME) \
		-f Dockerfile.turn \
		--load \
		.
	@rm -rf /tmp/.buildx-cache-turn
	@mv /tmp/.buildx-cache-turn-new /tmp/.buildx-cache-turn

aws-login:
	@echo "Logging into AWS ECR..."
	@aws ecr get-login-password --region $(AWS_REGION) | docker login --username AWS --password-stdin $(AWS_ACCOUNT_ID).dkr.ecr.$(AWS_REGION).amazonaws.com
	@aws ecr describe-repositories --repository-names $(ECR_TURN_REPO) || \
		aws ecr create-repository --repository-name $(ECR_TURN_REPO)

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

aws-push-turn: aws-login
	@echo "Building and pushing TURN server to ECR..."
	@docker pull $(ECR_TURN_URL):latest || true
	@mkdir -p /tmp/.buildx-cache-turn
	@docker buildx build \
		--platform $(PLATFORM) \
		--push \
		--cache-from type=local,src=/tmp/.buildx-cache-turn \
		--cache-from $(ECR_TURN_URL):latest \
		--cache-to type=local,dest=/tmp/.buildx-cache-turn-new \
		-t $(TURN_IMAGE_NAME):latest \
		-t $(ECR_TURN_URL):latest \
		-f Dockerfile.turn \
		.
	@docker push $(ECR_TURN_URL):latest
	@echo "Successfully pushed TURN server to ECR"
	@rm -rf /tmp/.buildx-cache-turn
	@mv /tmp/.buildx-cache-turn-new /tmp/.buildx-cache-turn

deploy-all: build build-turn aws-login
	@echo "Deploying all services..."
	# Tag TURN server image for ECR
	@docker tag $(TURN_IMAGE_NAME) $(ECR_TURN_URL):latest
	# Push TURN server to ECR
	@docker push $(ECR_TURN_URL):latest
	# Deploy infrastructure
	@cd infra && npm run deploy

test-generate-synth:
	curl -X POST \
		http://localhost:8080/generate-synth \
		-H "Content-Type: application/json" \
		-H "Awestruck-API-Key: $${AWESTRUCK_API_KEY}" \
		-d '{"prompt":"","provider":"openai","model":"o1-preview"}'