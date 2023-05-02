LOCAL_PREFIX = local
IMAGE = awestruck
AWS_ACCT = "${AWS_ACCT_ID}.dkr.ecr.us-east-1.amazonaws.com"
ECR_IMAGE = "${AWS_ACCT}/${IMAGE}"

.PHONY: ecr-login ecr-build update-ecr deploy-sbx build up down

	

# DEPLOYMENTS start ######
ecr-login:
	AWS_PROFILE=personal aws ecr get-login-password --region us-east-1 --profile $(AWS_PROFILE) | \
		docker login --username AWS --password-stdin $(AWS_ACCT)

ecr-build:
	docker buildx build \
		--tag "${LOCAL_PREFIX}-${IMAGE}" \
		--platform linux/amd64 \
		--load \
		--file Dockerfile .

update-ecr: ecr-build ecr-login
	docker tag "${LOCAL_PREFIX}-${IMAGE}":latest $(ECR_IMAGE):latest
	docker push $(ECR_IMAGE):latest

deploy-sbx: update-ecr
	eb deploy $(AWS_EB_ENV) --profile $(AWS_PROFILE)
# DEPLOYMENTS end ######



# LOCAL DEV start #####
build:
	go get github.com/po-studio/example-webrtc-applications/v3@1b18c4594b648ef48c2da02cfd65e8617e4fc2d8
	docker-compose -p $(LOCAL_PREFIX) -f docker-compose.yml build

up:
	docker-compose -p $(LOCAL_PREFIX) -f docker-compose.yml up

down:
	docker-compose -p $(LOCAL_PREFIX) down
# LOCAL DEV end #######