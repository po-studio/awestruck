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

.PHONY: build build-turn up down test-generate-synth aws-login aws-push aws-push-turn deploy-all deploy-infra build-all setup setup-ssl setup-hosts clean-ssl dev

# ---------------------------------------
# local dev only
# ---------------------------------------
export_host_ip:
	$(eval HOST_IP := $(shell ./scripts/get_host_ip.sh))
	@echo "Using host IP: $(HOST_IP)"

up: export_host_ip
	docker compose up

upb: export_host_ip
	docker compose up --build

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
		--load .
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
		-f Dockerfile.turn \
		--load .
	@rm -rf /tmp/.buildx-cache-turn
	@mv /tmp/.buildx-cache-turn-new /tmp/.buildx-cache-turn

aws-login:
	@echo "Logging into AWS ECR..."
	@aws ecr get-login-password --region $(AWS_REGION) | docker login --username AWS --password-stdin $(AWS_ACCOUNT_ID).dkr.ecr.$(AWS_REGION).amazonaws.com
	@for repo in $(ECR_WEBRTC_REPO) $(ECR_TURN_REPO); do \
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

deploy-all: build-all aws-push aws-push-turn deploy-infra

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
build-all: build build-turn
	@echo "All services built successfully"

# why we need optimized deployment:
# - ensures all services are built before pushing
# - maintains build cache across deployments
# - deploys infrastructure after images are ready
deploy-all: build-all aws-push aws-push-turn deploy-infra

# why we need phony targets:
# - prevents conflicts with files
# - ensures targets always run
# - improves makefile clarity
.PHONY: setup setup-ssl setup-hosts clean-ssl dev

# why we need setup dependencies:
# - ensures complete environment configuration
# - handles ssl and hosts in correct order
# - provides single command setup
setup: setup-ssl setup-hosts

# why we need ssl setup:
# - enables https for local development
# - matches production ssl termination
# - required for webrtc security
setup-ssl:
	@echo "Setting up SSL certificates..."
	@mkdir -p nginx/certs
	@if [ ! -f nginx/certs/localhost.crt ]; then \
		openssl req -x509 -nodes -days 365 -newkey rsa:2048 \
		-keyout nginx/certs/localhost.key \
		-out nginx/certs/localhost.crt \
		-subj "/C=US/ST=State/L=City/O=Organization/CN=localhost" \
		-addext "subjectAltName=DNS:localhost,DNS:turn.localhost"; \
		echo "SSL certificates generated successfully."; \
	else \
		echo "SSL certificates already exist."; \
	fi

# why we need hosts setup:
# - enables local domain resolution
# - simulates aws dns setup
# - required for turn server discovery
setup-hosts:
	@echo "Setting up /etc/hosts entries..."
	@if ! grep -q "turn.localhost" /etc/hosts; then \
		echo "Adding turn.localhost to /etc/hosts..."; \
		echo "127.0.0.1 turn.localhost" | sudo tee -a /etc/hosts; \
	else \
		echo "turn.localhost already in /etc/hosts"; \
	fi

# why we need ssl cleanup:
# - enables certificate regeneration
# - removes old certificates
# - helps troubleshoot ssl issues
clean-ssl:
	@echo "Cleaning SSL certificates..."
	@rm -rf nginx/certs

# why we need dev environment:
# - starts all required services
# - ensures proper initialization
# - simplifies local development
dev: setup
	docker-compose up --build