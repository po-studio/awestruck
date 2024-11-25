# Docker Configuration
PLATFORM ?= linux/arm64
IMAGE_NAME=go-webrtc-server-$(subst /,-,$(PLATFORM))
CONTAINER_NAME=$(IMAGE_NAME)-instance
BROWSER_PORT=8080

# AWS-related variables
AWS_ACCOUNT_ID ?= $(shell aws sts get-caller-identity --query Account --output text)
AWS_REGION ?= us-east-1
ECR_REPO = po-studio/awestruck
ECR_URL = $(AWS_ACCOUNT_ID).dkr.ecr.$(AWS_REGION).amazonaws.com

.PHONY: build up down rshell shell open-browser aws-login aws-push

build:
	$(DOCKER_COMPOSE) build

up:
	$(DOCKER_COMPOSE) up -d

down:
	$(DOCKER_COMPOSE) down

rshell:
	$(DOCKER_COMPOSE) run --rm $(IMAGE_NAME) /bin/sh

shell:
	$(DOCKER_COMPOSE) exec $(IMAGE_NAME) /bin/sh

open-browser:
	open http://localhost:$(BROWSER_PORT)

aws-login:
	aws ecr get-login-password --region $(AWS_REGION) | docker login --username AWS --password-stdin $(ECR_URL)

aws-push: aws-login
	docker tag $(IMAGE_NAME):latest $(ECR_URL)/$(ECR_REPO):latest
	docker push $(ECR_URL)/$(ECR_REPO):latest

synth:
	cd infra && cdktf synth

deploy: synth
	cd infra && cdktf deploy
