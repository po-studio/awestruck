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
ECR_WEBRTC_URL = $(AWS_ACCOUNT_ID).dkr.ecr.$(AWS_REGION).amazonaws.com/$(ECR_WEBRTC_REPO)
ECR_TURN_REPO = po-studio/awestruck/services/turn
ECR_TURN_URL = $(AWS_ACCOUNT_ID).dkr.ecr.$(AWS_REGION).amazonaws.com/$(ECR_TURN_REPO)
ECR_CLIENT_REPO = po-studio/awestruck/services/client
ECR_CLIENT_URL = $(AWS_ACCOUNT_ID).dkr.ecr.$(AWS_REGION).amazonaws.com/$(ECR_CLIENT_REPO)

.PHONY: build build-turn build-client up down test-generate-synth aws-login aws-push aws-push-turn aws-push-client deploy-all deploy-infra build-all

# ---------------------------------------
# local dev only
# ---------------------------------------
export_host_ip:
	$(eval HOST_IP := $(shell ./scripts/get_dev_host_ip.sh))
	@echo "Using host IP: $(HOST_IP)"

up: export_host_ip
	docker compose up

upb: export_host_ip
	NODE_ENV=development docker compose up --build

down:
	docker-compose down --remove-orphans

r: down upb

# ---------------------------------------

# ---------------------------------------
# deployment build, push, and deploy
# ---------------------------------------
build:
	@echo "Building WebRTC server..."
	@if ! docker buildx inspect mybuilder > /dev/null 2>&1; then \
		docker buildx create --use --name mybuilder; \
	else \
		docker buildx use mybuilder; \
	fi
	@docker buildx inspect --bootstrap
	@docker pull $(ECR_WEBRTC_URL):latest || true
	@mkdir -p /tmp/.buildx-cache
	@DOCKER_BUILDKIT=1 docker buildx build \
		--platform $(PLATFORM) \
		--cache-from type=local,src=/tmp/.buildx-cache \
		--cache-from $(ECR_WEBRTC_URL):latest \
		--cache-to type=local,dest=/tmp/.buildx-cache-new \
		-t $(IMAGE_NAME):latest \
		-f server/Dockerfile server \
		--load
	@rm -rf /tmp/.buildx-cache
	@mv /tmp/.buildx-cache-new /tmp/.buildx-cache

build-turn:
	@echo "Building TURN server..."
	@if ! docker buildx inspect mybuilder > /dev/null 2>&1; then \
		docker buildx create --use --name mybuilder; \
	else \
		docker buildx use mybuilder; \
	fi
	@docker buildx inspect --bootstrap
	@docker pull $(ECR_TURN_URL):latest || true
	@mkdir -p /tmp/.buildx-cache-turn
	@docker buildx build \
		--platform $(PLATFORM) \
		--cache-from type=local,src=/tmp/.buildx-cache-turn \
		--cache-from $(ECR_TURN_URL):latest \
		--cache-to type=local,dest=/tmp/.buildx-cache-turn-new \
		-t $(TURN_IMAGE_NAME):latest \
		-f turn/Dockerfile turn \
		--load
	@rm -rf /tmp/.buildx-cache-turn
	@mv /tmp/.buildx-cache-turn-new /tmp/.buildx-cache-turn

build-client:
	@echo "Building client application..."
	@if ! docker buildx inspect mybuilder > /dev/null 2>&1; then \
		docker buildx create --use --name mybuilder; \
	else \
		docker buildx use mybuilder; \
	fi
	@docker buildx inspect --bootstrap
	@docker pull $(ECR_CLIENT_URL):latest || true
	@mkdir -p /tmp/.buildx-cache-client
	@docker buildx build \
		--platform $(PLATFORM) \
		--cache-from type=local,src=/tmp/.buildx-cache-client \
		--cache-from $(ECR_CLIENT_URL):latest \
		--cache-to type=local,dest=/tmp/.buildx-cache-client-new \
		-t client:latest \
		-f client/Dockerfile client \
		--load
	@rm -rf /tmp/.buildx-cache-client
	@mv /tmp/.buildx-cache-client-new /tmp/.buildx-cache-client

aws-login:
	@echo "Logging into AWS ECR..."
	@aws ecr get-login-password --region $(AWS_REGION) | docker login --username AWS --password-stdin $(AWS_ACCOUNT_ID).dkr.ecr.$(AWS_REGION).amazonaws.com
	@for repo in $(ECR_WEBRTC_REPO) $(ECR_TURN_REPO) $(ECR_CLIENT_REPO); do \
		echo "Checking repository $$repo..."; \
		aws ecr describe-repositories --repository-names $$repo || \
		(echo "Creating repository $$repo..." && \
		aws ecr create-repository --repository-name $$repo); \
	done

aws-push: aws-login
	@echo "Pushing WebRTC server to ECR..."
	# Tag the already built image
	docker tag $(IMAGE_NAME):latest $(ECR_WEBRTC_URL):latest
	# Push to ECR
	docker push $(ECR_WEBRTC_URL):latest

aws-push-turn: aws-login
	@echo "Pushing TURN server to ECR..."
	# Tag the already built image
	docker tag $(TURN_IMAGE_NAME):latest $(ECR_TURN_URL):latest
	# Push to ECR
	docker push $(ECR_TURN_URL):latest

aws-push-client: aws-login
	@echo "Pushing client to ECR..."
	# Tag the already built image
	docker tag client:latest $(ECR_CLIENT_URL):latest
	# Push to ECR
	docker push $(ECR_CLIENT_URL):latest

deploy-all: build-all aws-push aws-push-turn aws-push-client deploy-infra

deploy-infra:
	@echo "Deploying infrastructure..."
	@cd infra && npm install && npm run deploy

test-generate-synth:
	curl -X POST \
		http://localhost:8080/generate-synth \
		-H "Content-Type: application/json" \
		-H "Awestruck-API-Key: $${AWESTRUCK_API_KEY}" \
		-d '{"prompt":"","provider":"openai","model":"o1-preview"}'

# why we need optimized build:
# - uses buildx for multi-platform support
# - maintains local and remote cache
# - ensures consistent image tags
build-all: build build-turn build-client
	@echo "All services built successfully"

# why we need optimized deployment:
# - ensures all services are built before pushing
# - maintains build cache across deployments
# - deploys infrastructure after images are ready
deploy-all: build-all aws-push aws-push-turn aws-push-client deploy-infra