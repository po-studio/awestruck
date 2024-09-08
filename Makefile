# Docker Configuration
PLATFORM ?= linux/arm64
IMAGE_NAME=go-webrtc-server-$(subst /,-,$(PLATFORM))
CONTAINER_NAME=$(IMAGE_NAME)-instance
BROWSER_PORT=8080

# AWS-related variables
AWS_ACCOUNT_ID ?= $(shell aws sts get-caller-identity --query Account --output text)
AWS_REGION ?= us-east-1
TASK_DEFINITION_FAMILY ?= go-webrtc-server-arm64
EXECUTION_ROLE_ARN := arn:aws:iam::$(AWS_ACCOUNT_ID):role/ecsTaskExecutionRole

# AWS and Project Configuration
PROJECT_NAME ?= awestruck
DOMAIN_NAME ?= awestruck.io
SSL_CERT_ARN ?= arn:aws:acm:$(AWS_REGION):$(AWS_ACCOUNT_ID):certificate/3fa50879-056c-46a9-9ad5-74af71d719d7

# ECS Configuration
ECS_CLUSTER ?= awestruck
ECS_SERVICE_NAME ?= $(PROJECT_NAME)-service

# ECR Configuration
ECR_REPO = po-studio/awestruck
ECR_URL = $(AWS_ACCOUNT_ID).dkr.ecr.$(AWS_REGION).amazonaws.com

# Target Group ARN
TG_ARN := $(shell aws elbv2 describe-target-groups --names $(PROJECT_NAME)-tg --query 'TargetGroups[0].TargetGroupArn' --output text)

# Security Group ID
# SG_ID := $(shell aws ec2 describe-security-groups --filters "Name=group-name,Values=$(PROJECT_NAME)-sg" --query 'SecurityGroups[0].GroupId' --output text)
VPC_ID := $(shell aws ec2 describe-vpcs --filters "Name=tag:Name,Values=$(PROJECT_NAME)-vpc" --query 'Vpcs[0].VpcId' --output text)
SG_ID := $(shell aws ec2 describe-security-groups --filters "Name=vpc-id,Values=$(VPC_ID)" "Name=group-name,Values=$(PROJECT_NAME)-sg" --query 'SecurityGroups[0].GroupId' --output text)
SUBNET1_ID ?= $(shell aws ec2 describe-subnets --filters "Name=tag:Name,Values=$(PROJECT_NAME)-subnet-1" --query 'Subnets[0].SubnetId' --output text)
SUBNET2_ID ?= $(shell aws ec2 describe-subnets --filters "Name=tag:Name,Values=$(PROJECT_NAME)-subnet-2" --query 'Subnets[0].SubnetId' --output text)



# ALB ARN
ALB_ARN := $(shell aws elbv2 describe-load-balancers --names $(PROJECT_NAME)-alb --query 'LoadBalancers[0].LoadBalancerArn' --output text)

.PHONY: build up down rshell shell open-browser aws-login aws-push aws-deploy setup-infrastructure deploy check-ecs-task check-logs check-security-group check-target-health check-alb-listener debug-logs check-stopped-tasks check-ecs-service update-ecs-service check-port-mappings check-target-group update-security-group update-target-group check-task-definition inspect-unhealthy-target

build:
	@if ! docker buildx inspect mybuilder > /dev/null 2>&1; then \
		docker buildx create --use --name mybuilder; \
	else \
		docker buildx use mybuilder; \
	fi
	docker buildx inspect --bootstrap
	docker buildx build --platform $(PLATFORM) -t $(IMAGE_NAME) --load .

up: 
	docker run --rm --name $(CONTAINER_NAME) \
		--platform $(PLATFORM) \
		-p $(BROWSER_PORT):$(BROWSER_PORT) \
		--network host \
		--shm-size=1g \
		--ulimit memlock=-1 \
		--ulimit stack=67108864 \
		-e JACK_NO_AUDIO_RESERVATION=1 \
		-e JACK_OPTIONS="-R -d dummy" \
		-e JACK_SAMPLE_RATE=48000 \
		$(IMAGE_NAME)

down:
	@echo "Stopping the container..."
	-docker stop $(CONTAINER_NAME)
	@echo "Stopping SuperCollider server..."
	-docker exec -it $(CONTAINER_NAME) pkill scsynth || true
	@echo "Everything stopped gracefully."

rshell:
	@CONTAINER_ID=$$(docker ps --filter "ancestor=$(IMAGE_NAME)" --format "{{.ID}}" | head -n 1); \
	if [ -n "$$CONTAINER_ID" ]; then \
		docker exec -u root -it $$CONTAINER_ID /bin/bash; \
	else \
		echo "No running container found for '$(IMAGE_NAME)'."; \
	fi

shell:
	@CONTAINER_ID=$$(docker ps --filter "ancestor=$(IMAGE_NAME)" --format "{{.ID}}" | head -n 1); \
	if [ -n "$$CONTAINER_ID" ]; then \
		docker exec -it $$CONTAINER_ID /bin/bash; \
	else \
		echo "No running container found for '$(IMAGE_NAME)'."; \
	fi

open-browser:
	@-if command -v xdg-open > /dev/null; then \
		xdg-open http://localhost:$(BROWSER_PORT); \
	elif command -v open > /dev/null; then \
		open http://localhost:$(BROWSER_PORT); \
	elif command -v cmd /c start > /dev/null; then \
		cmd /c start http://localhost:$(BROWSER_PORT); \
	else \
		echo "Please open http://localhost:$(BROWSER_PORT) in your browser."; \
	fi

aws-login:
	aws ecr get-login-password --region $(AWS_REGION) | docker login --username AWS --password-stdin $(ECR_URL)

aws-push: aws-login
	docker tag $(IMAGE_NAME) $(ECR_URL)/$(ECR_REPO):latest
	docker push $(ECR_URL)/$(ECR_REPO):latest

# aws-deploy: aws-push update-security-group-all-ports
# 	@echo "Updating ECS task definition..."
# 	sed 's|{{AWS_ACCOUNT_ID}}|$(AWS_ACCOUNT_ID)|g; s|{{AWS_REGION}}|$(AWS_REGION)|g; s|{{ECR_REPO}}|$(ECR_REPO)|g; s|{{SSL_CERT_ARN}}|$(SSL_CERT_ARN)|g; s|{{EXECUTION_ROLE_ARN}}|$(EXECUTION_ROLE_ARN)|g; s|{{TASK_DEFINITION_FAMILY}}|$(TASK_DEFINITION_FAMILY)|g' aws/task-definition.json > aws/task-definition-filled.json
# 	aws ecs register-task-definition --cli-input-json file://aws/task-definition-filled.json

# 	@echo "Updating ECS service..."
# 	aws ecs update-service --cluster $(ECS_CLUSTER) --service $(ECS_SERVICE_NAME) --task-definition $(TASK_DEFINITION_FAMILY) --force-new-deployment

aws-deploy: aws-push update-security-group-all-ports
	@echo "Updating ECS task definition..."
	sed 's|{{AWS_ACCOUNT_ID}}|$(AWS_ACCOUNT_ID)|g; \
		s|{{AWS_REGION}}|$(AWS_REGION)|g; \
		s|{{ECR_REPO}}|$(ECR_REPO)|g; \
		s|{{EXECUTION_ROLE_ARN}}|$(EXECUTION_ROLE_ARN)|g; \
		s|{{TASK_DEFINITION_FAMILY}}|$(TASK_DEFINITION_FAMILY)|g; \
		s|{{PROJECT_NAME}}|$(PROJECT_NAME)|g; \
		s|{{SG_ID}}|$(SG_ID)|g; \
		s|{{SUBNET1_ID}}|$(SUBNET1_ID)|g; \
		s|{{SUBNET2_ID}}|$(SUBNET2_ID)|g' \
		aws/task-definition.json > aws/task-definition-filled.json
	aws ecs register-task-definition --cli-input-json file://aws/task-definition-filled.json

	@echo "Updating ECS service..."
	aws ecs update-service --cluster $(ECS_CLUSTER) --service $(ECS_SERVICE_NAME) --task-definition $(TASK_DEFINITION_FAMILY) --force-new-deployment

setup-infrastructure:
	@echo "Setting up infrastructure..."

	@echo "Checking/Creating ecsTaskExecutionRole..."
	@if ! aws iam get-role --role-name ecsTaskExecutionRole > /dev/null 2>&1; then \
		echo "ecsTaskExecutionRole does not exist. Creating..."; \
		aws iam create-role --role-name ecsTaskExecutionRole --assume-role-policy-document file://aws/trust-policy.json > /dev/null && \
		aws iam attach-role-policy --role-name ecsTaskExecutionRole --policy-arn arn:aws:iam::aws:policy/service-role/AmazonECSTaskExecutionRolePolicy > /dev/null && \
		echo "ecsTaskExecutionRole created and policy attached."; \
	else \
		echo "ecsTaskExecutionRole already exists. Updating trust relationship..."; \
		aws iam update-assume-role-policy --role-name ecsTaskExecutionRole --policy-document file://aws/trust-policy.json > /dev/null; \
	fi

	@echo "Attaching CloudWatchLogsFullAccess policy to ecsTaskExecutionRole..."
	aws iam attach-role-policy --role-name ecsTaskExecutionRole --policy-arn arn:aws:iam::aws:policy/CloudWatchLogsFullAccess

	@echo "Creating CloudWatch log group..."
	aws logs create-log-group --log-group-name "/ecs/$(TASK_DEFINITION_FAMILY)" --region $(AWS_REGION) || true

	@echo "Ensuring IAM user/role has necessary permissions..."
	$(eval CURRENT_USER := $(shell aws sts get-caller-identity --query 'Arn' --output text))
	@echo "Current AWS CLI identity: $(CURRENT_USER)"
	@if echo "$(CURRENT_USER)" | grep -q "root"; then \
		echo "Using root account. No need to add additional permissions."; \
	else \
		USER_NAME=$$(echo "$(CURRENT_USER)" | rev | cut -d'/' -f1 | rev); \
		if ! aws iam get-user-policy --user-name "$$USER_NAME" --policy-name EcsTaskExecutionRolePass > /dev/null 2>&1; then \
			echo "Adding permission to pass ecsTaskExecutionRole..."; \
			aws iam put-user-policy --user-name "$$USER_NAME" \
				--policy-name EcsTaskExecutionRolePass \
				--policy-document file://aws/pass-role-policy.json > /dev/null; \
		else \
			echo "Permission to pass ecsTaskExecutionRole already exists."; \
		fi; \
	fi

	@echo "Checking/Creating ECS cluster..."
	@if ! aws ecs describe-clusters --clusters $(ECS_CLUSTER) --query 'clusters[0].clusterArn' --output text | grep -q $(ECS_CLUSTER); then \
		echo "Creating ECS cluster $(ECS_CLUSTER)..."; \
		aws ecs create-cluster --cluster-name $(ECS_CLUSTER) > /dev/null; \
	else \
		echo "ECS cluster $(ECS_CLUSTER) already exists."; \
	fi

	@echo "Checking/Creating VPC..."
	$(eval VPC_ID := $(shell aws ec2 describe-vpcs --filters "Name=tag:Name,Values=$(PROJECT_NAME)-vpc" --query 'Vpcs[0].VpcId' --output text))
	@if [ "$(VPC_ID)" = "None" ]; then \
		VPC_ID=$$(aws ec2 create-vpc --cidr-block 10.0.0.0/16 --query 'Vpc.VpcId' --output text); \
		aws ec2 create-tags --resources $$VPC_ID --tags Key=Name,Value=$(PROJECT_NAME)-vpc; \
		aws ec2 modify-vpc-attribute --vpc-id $$VPC_ID --enable-dns-hostnames; \
		echo "VPC created: $$VPC_ID"; \
	else \
		echo "Using existing VPC: $(VPC_ID)"; \
	fi

	@echo "Checking/Creating Internet Gateway..."
	$(eval IGW_ID := $(shell aws ec2 describe-internet-gateways --filters "Name=attachment.vpc-id,Values=$(VPC_ID)" --query 'InternetGateways[0].InternetGatewayId' --output text))
	@if [ "$(IGW_ID)" = "None" ]; then \
		IGW_ID=$$(aws ec2 create-internet-gateway --query 'InternetGateway.InternetGatewayId' --output text); \
		aws ec2 attach-internet-gateway --vpc-id $(VPC_ID) --internet-gateway-id $$IGW_ID; \
		echo "Internet Gateway created and attached: $$IGW_ID"; \
	else \
		echo "Using existing Internet Gateway: $(IGW_ID)"; \
	fi

	@echo "Checking/Creating subnets..."
	$(eval SUBNET1_ID := $(shell aws ec2 describe-subnets --filters "Name=vpc-id,Values=$(VPC_ID)" "Name=availability-zone,Values=$(AWS_REGION)a" --query 'Subnets[0].SubnetId' --output text))
	$(eval SUBNET2_ID := $(shell aws ec2 describe-subnets --filters "Name=vpc-id,Values=$(VPC_ID)" "Name=availability-zone,Values=$(AWS_REGION)b" --query 'Subnets[0].SubnetId' --output text))
	@if [ "$(SUBNET1_ID)" = "None" ]; then \
		SUBNET1_ID=$$(aws ec2 create-subnet --vpc-id $(VPC_ID) --cidr-block 10.0.1.0/24 --availability-zone $(AWS_REGION)a --query 'Subnet.SubnetId' --output text); \
		aws ec2 create-tags --resources $$SUBNET1_ID --tags Key=Name,Value=$(PROJECT_NAME)-subnet-1; \
		echo "Subnet 1 created: $$SUBNET1_ID"; \
	else \
		echo "Using existing Subnet 1: $(SUBNET1_ID)"; \
	fi
	@if [ "$(SUBNET2_ID)" = "None" ]; then \
		SUBNET2_ID=$$(aws ec2 create-subnet --vpc-id $(VPC_ID) --cidr-block 10.0.2.0/24 --availability-zone $(AWS_REGION)b --query 'Subnet.SubnetId' --output text); \
		aws ec2 create-tags --resources $$SUBNET2_ID --tags Key=Name,Value=$(PROJECT_NAME)-subnet-2; \
		echo "Subnet 2 created: $$SUBNET2_ID"; \
	else \
		echo "Using existing Subnet 2: $(SUBNET2_ID)"; \
	fi

	@echo "Checking/Creating route table..."
	$(eval ROUTE_TABLE_ID := $(shell aws ec2 describe-route-tables --filters "Name=vpc-id,Values=$(VPC_ID)" "Name=association.main,Values=false" --query 'RouteTables[0].RouteTableId' --output text))
	@if [ "$(ROUTE_TABLE_ID)" = "None" ]; then \
		ROUTE_TABLE_ID=$$(aws ec2 create-route-table --vpc-id $(VPC_ID) --query 'RouteTable.RouteTableId' --output text); \
		aws ec2 create-route --route-table-id $$ROUTE_TABLE_ID --destination-cidr-block 0.0.0.0/0 --gateway-id $(IGW_ID) > /dev/null; \
		aws ec2 associate-route-table --subnet-id $(SUBNET1_ID) --route-table-id $$ROUTE_TABLE_ID > /dev/null; \
		aws ec2 associate-route-table --subnet-id $(SUBNET2_ID) --route-table-id $$ROUTE_TABLE_ID > /dev/null; \
		echo "Route table created and associated: $$ROUTE_TABLE_ID"; \
	else \
		echo "Using existing route table: $(ROUTE_TABLE_ID)"; \
	fi

	@echo "Checking/Creating security group..."
	$(eval SG_ID := $(shell aws ec2 describe-security-groups --filters "Name=vpc-id,Values=$(VPC_ID)" "Name=group-name,Values=$(PROJECT_NAME)-sg" --query 'SecurityGroups[0].GroupId' --output text))
	@if [ "$(SG_ID)" = "None" ]; then \
		SG_ID=$$(aws ec2 create-security-group --group-name $(PROJECT_NAME)-sg --description "Security group for $(PROJECT_NAME)" --vpc-id $(VPC_ID) --query 'GroupId' --output text); \
		aws ec2 authorize-security-group-ingress --group-id $$SG_ID --protocol tcp --port 8080 --cidr 0.0.0.0/0 > /dev/null; \
		aws ec2 authorize-security-group-ingress --group-id $$SG_ID --protocol tcp --port 443 --cidr 0.0.0.0/0 > /dev/null; \
		echo "Security group created: $$SG_ID"; \
	else \
		echo "Using existing security group: $(SG_ID)"; \
	fi

	@echo "Checking/Creating ALB..."
	$(eval ALB_ARN := $(shell aws elbv2 describe-load-balancers --names $(PROJECT_NAME)-alb --query 'LoadBalancers[0].LoadBalancerArn' --output text 2>/dev/null))
	@if [ "$(ALB_ARN)" = "None" ] || [ -z "$(ALB_ARN)" ]; then \
		echo "Creating new ALB..."; \
		ALB_ARN=$$(aws elbv2 create-load-balancer --name $(PROJECT_NAME)-alb --subnets $(SUBNET1_ID) $(SUBNET2_ID) --security-groups $(SG_ID) --scheme internet-facing --type application --query 'LoadBalancers[0].LoadBalancerArn' --output text); \
		echo "ALB created: $$ALB_ARN"; \
	else \
		echo "Using existing ALB: $(ALB_ARN)"; \
	fi
	@echo "ALB_ARN: $(ALB_ARN)"

	@echo "Checking/Creating target group..."
	$(eval TG_ARN := $(shell aws elbv2 describe-target-groups --names $(PROJECT_NAME)-tg --query 'TargetGroups[0].TargetGroupArn' --output text 2>/dev/null))
	@if [ "$(TG_ARN)" = "None" ] || [ -z "$(TG_ARN)" ]; then \
		echo "Creating new target group..."; \
		TG_ARN=$$(aws elbv2 create-target-group --name $(PROJECT_NAME)-tg --protocol HTTP --port 8080 --vpc-id $(VPC_ID) --target-type ip --health-check-path / --health-check-interval-seconds 30 --query 'TargetGroups[0].TargetGroupArn' --output text); \
		echo "Target group created: $$TG_ARN"; \
	else \
		echo "Using existing target group: $(TG_ARN)"; \
	fi
	@echo "TG_ARN: $(TG_ARN)"

	@echo "Checking/Creating HTTPS listener..."
	$(eval LISTENER_ARN := $(shell aws elbv2 describe-listeners --load-balancer-arn $(ALB_ARN) --query 'Listeners[?Protocol==`HTTPS`].ListenerArn' --output text 2>/dev/null))
	@if [ "$(LISTENER_ARN)" = "None" ] || [ -z "$(LISTENER_ARN)" ]; then \
		echo "Creating HTTPS listener..."; \
		aws elbv2 create-listener --load-balancer-arn $(ALB_ARN) --protocol HTTPS --port 443 --certificates CertificateArn=$(SSL_CERT_ARN) --default-actions Type=forward,TargetGroupArn=$(TG_ARN) > /dev/null; \
		echo "HTTPS listener created"; \
	else \
		echo "HTTPS listener already exists"; \
	fi

	@echo "Checking/Creating ECS service..."
	$(eval SERVICE_ARN := $(shell aws ecs describe-services --cluster $(ECS_CLUSTER) --services $(ECS_SERVICE_NAME) --query 'services[0].serviceArn' --output text 2>/dev/null))
	@if [ "$(SERVICE_ARN)" = "None" ] || [ -z "$(SERVICE_ARN)" ]; then \
		echo "Creating ECS service..."; \
		aws ecs create-service \
			--cluster $(ECS_CLUSTER) \
			--service-name $(ECS_SERVICE_NAME) \
			--task-definition $(TASK_DEFINITION_FAMILY) \
			--desired-count 1 \
			--launch-type FARGATE \
			--network-configuration "awsvpcConfiguration={subnets=[$(SUBNET1_ID),$(SUBNET2_ID)],securityGroups=[$(SG_ID)],assignPublicIp=ENABLED}" \
			--load-balancers "targetGroupArn=$(TG_ARN),containerName=$(TASK_DEFINITION_FAMILY),containerPort=8080" || \
		echo "Failed to create ECS service. You may need to create it manually."; \
	else \
		echo "ECS service already exists: $(SERVICE_ARN)"; \
		echo "Updating ECS service..."; \
		aws ecs update-service --cluster $(ECS_CLUSTER) --service $(ECS_SERVICE_NAME) --task-definition $(TASK_DEFINITION_FAMILY) --force-new-deployment > /dev/null || echo "Failed to update ECS service"; \
	fi

	@echo "Checking task status..."
	$(eval TASK_ARN := $(shell aws ecs list-tasks --cluster $(ECS_CLUSTER) --service-name $(ECS_SERVICE_NAME) --query 'taskArns[0]' --output text))
	@if [ "$(TASK_ARN)" = "None" ] || [ -z "$(TASK_ARN)" ]; then \
		echo "No running tasks found. Checking for stopped tasks..."; \
		STOPPED_TASK=$$(aws ecs list-tasks --cluster $(ECS_CLUSTER) --service-name $(ECS_SERVICE_NAME) --desired-status STOPPED --query 'taskArns[0]' --output text); \
		if [ "$$STOPPED_TASK" != "None" ] && [ -n "$$STOPPED_TASK" ]; then \
			echo "Found stopped task. Fetching reason:"; \
			aws ecs describe-tasks --cluster $(ECS_CLUSTER) --tasks $$STOPPED_TASK --query 'tasks[0].stoppedReason' --output text; \
			echo "Container status:"; \
			aws ecs describe-tasks --cluster $(ECS_CLUSTER) --tasks $$STOPPED_TASK --query 'tasks[0].containers[0].reason' --output text; \
		else \
			echo "No stopped tasks found. The service might be having trouble starting tasks."; \
		fi; \
	else \
		echo "Task is running: $(TASK_ARN)"; \
	fi

	@echo "Checking/Creating Route 53 record..."
	$(eval HOSTED_ZONE_ID := $(shell aws route53 list-hosted-zones-by-name --dns-name $(DOMAIN_NAME) --query 'HostedZones[0].Id' --output text | sed 's/\/hostedzone\///'))
	$(eval ALB_DNS_NAME := $(shell aws elbv2 describe-load-balancers --load-balancer-arns $(ALB_ARN) --query 'LoadBalancers[0].DNSName' --output text))
	$(eval ALB_HOSTED_ZONE_ID := $(shell aws elbv2 describe-load-balancers --load-balancer-arns $(ALB_ARN) --query 'LoadBalancers[0].CanonicalHostedZoneId' --output text))
	@if [ -n "$(ALB_DNS_NAME)" ] && [ -n "$(ALB_HOSTED_ZONE_ID)" ]; then \
		aws route53 change-resource-record-sets --hosted-zone-id $(HOSTED_ZONE_ID) --change-batch '{ "Changes": [{ "Action": "UPSERT", "ResourceRecordSet": { "Name": "$(DOMAIN_NAME)", "Type": "A", "AliasTarget": { "HostedZoneId": "$(ALB_HOSTED_ZONE_ID)", "DNSName": "$(ALB_DNS_NAME)", "EvaluateTargetHealth": true }}}]}' > /dev/null; \
		echo "Route 53 record created or updated"; \
	else \
		echo "Failed to update Route 53 record: ALB DNS name or Hosted Zone ID not found"; \
	fi

	@echo "Infrastructure setup complete!"

deploy: aws-deploy
	@echo "Deployment complete!"

check-ecs-task:
	@echo "Checking ECS task status..."
	aws ecs list-tasks --cluster $(ECS_CLUSTER) --service-name $(ECS_SERVICE_NAME) --query 'taskArns[0]' --output text | xargs -I {} aws ecs describe-tasks --cluster $(ECS_CLUSTER) --tasks {} --query 'tasks[0].lastStatus' --output text

check-logs:
	@echo "Fetching recent CloudWatch logs..."
	@if aws logs describe-log-streams --log-group-name "/ecs/$(TASK_DEFINITION_FAMILY)" --limit 1 --query 'logStreams[0].logStreamName' --output text > /dev/null 2>&1; then \
		aws logs get-log-events --log-group-name "/ecs/$(TASK_DEFINITION_FAMILY)" --log-stream-name $$(aws logs describe-log-streams --log-group-name "/ecs/$(TASK_DEFINITION_FAMILY)" --order-by LastEventTime --descending --limit 1 --query 'logStreams[0].logStreamName' --output text) --limit 20 --query 'events[*].message' --output text; \
	else \
		echo "No log streams found in the log group."; \
	fi

update-security-group:
	@echo "Updating security group rules for WebRTC..."
	@aws ec2 authorize-security-group-ingress \
		--group-id $(SG_ID) \
		--protocol udp \
		--port 10000-65535 \
		--cidr 0.0.0.0/0 \
		--region $(AWS_REGION) || true
	@echo "Security group updated for WebRTC ports."

update-security-group-all-ports:
	@echo "Updating security group to allow all inbound traffic (CAUTION: This is insecure!)"
	@aws ec2 authorize-security-group-ingress \
		--group-id $(SG_ID) \
		--protocol all \
		--port -1 \
		--cidr 0.0.0.0/0 \
		--region $(AWS_REGION) || true
	@echo "Security group updated to allow all inbound traffic."

check-target-group:
	@echo "Checking target group configuration..."
	aws elbv2 describe-target-group-attributes --target-group-arn $(TG_ARN)
	@echo "Checking health check configuration..."
	aws elbv2 describe-target-groups --target-group-arns $(TG_ARN) --query 'TargetGroups[0].HealthCheckPath'

update-target-group:
	@echo "Updating target group health check..."
	aws elbv2 modify-target-group --target-group-arn $(TG_ARN) --health-check-path / --health-check-interval-seconds 30

check-security-group:
	@echo "Checking security group rules..."
	@echo "Security Group ID: $(SG_ID)"
	@if [ -z "$(SG_ID)" ]; then \
		echo "Error: Security Group ID is empty. Check if the security group $(PROJECT_NAME)-sg exists."; \
	else \
		aws ec2 describe-security-groups --group-ids $(SG_ID) --query 'SecurityGroups[0].IpPermissions' --output table; \
	fi

check-alb-listener:
	@echo "Checking ALB listener..."
	@echo "ALB ARN: $(ALB_ARN)"
	@if [ -z "$(ALB_ARN)" ]; then \
		echo "Error: ALB ARN is empty. Check if the ALB $(PROJECT_NAME)-alb exists."; \
	else \
		aws elbv2 describe-listeners --load-balancer-arn $(ALB_ARN) --query 'Listeners[*].[Protocol,Port,DefaultActions[0].Type,DefaultActions[0].TargetGroupArn]' --output table; \
	fi

check-task-definition:
	@echo "Checking task definition..."
	aws ecs describe-task-definition --task-definition $(TASK_DEFINITION_FAMILY) --query 'taskDefinition.containerDefinitions[0].portMappings'

inspect-unhealthy-target:
	@echo "Inspecting unhealthy target..."
	@UNHEALTHY_TARGET=$$(aws elbv2 describe-target-health --target-group-arn $(TG_ARN) --query 'TargetHealthDescriptions[?TargetHealth.State==`unhealthy`].Target.Id' --output text | head -n1); \
	if [ -n "$$UNHEALTHY_TARGET" ]; then \
		aws elbv2 describe-target-health --target-group-arn $(TG_ARN) --targets Id=$$UNHEALTHY_TARGET --query 'TargetHealthDescriptions[0].TargetHealth'; \
	else \
		echo "No unhealthy targets found."; \
	fi

update-ecs-service:
	@echo "Updating ECS service..."
	@LATEST_TASK_DEF=$$(aws ecs describe-task-definition --task-definition $(TASK_DEFINITION_FAMILY) --query 'taskDefinition.taskDefinitionArn' --output text); \
	aws ecs update-service --cluster $(ECS_CLUSTER) --service $(ECS_SERVICE_NAME) --task-definition $$LATEST_TASK_DEF --force-new-deployment

check-port-mappings:
	@echo "Checking container port mappings..."
	aws ecs describe-task-definition --task-definition $(TASK_DEFINITION_FAMILY) --query 'taskDefinition.containerDefinitions[0].portMappings'

check-iam-role-trust:
	@echo "Checking IAM role trust policy..."
	aws iam get-role --role-name ecsTaskExecutionRole --query 'Role.AssumeRolePolicyDocument' --output json

check-iam-role-policies:
	@echo "Checking IAM role attached policies..."
	aws iam list-attached-role-policies --role-name ecsTaskExecutionRole --query 'AttachedPolicies[*].PolicyName' --output table


check-and-update-security-group:
	@echo "Checking and updating security group..."
	@if ! aws ec2 describe-security-groups --group-ids $(SG_ID) --query 'SecurityGroups[0].IpPermissions[?FromPort==`8080`]' --output text | grep -q 8080; then \
		echo "Adding rule for port 8080..."; \
		aws ec2 authorize-security-group-ingress --group-id $(SG_ID) --protocol tcp --port 8080 --cidr 0.0.0.0/0; \
	else \
		echo "Rule for port 8080 already exists."; \
	fi