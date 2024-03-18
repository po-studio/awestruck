# Makefile for building and running a Docker container based on your Docker Compose configuration

IMAGE_NAME=go-webrtc-server
CONTAINER_NAME=go-webrtc-server-instance

.PHONY: build run

build:
	docker build -t $(IMAGE_NAME) .

run:
	docker run --rm --name $(CONTAINER_NAME) \
		--user "1000" \
		--privileged \
		--ulimit memlock=-1:-1 \
		--ulimit rtprio=99:99 \
		--cap-add SYS_NICE \
		-p 8080:8080 \
		-p 3033:3033 \
		-p 3478:3478/udp \
		-p 3478:3478/tcp \
		-p 5349:5349/tcp \
		-p 50000:50000 \
		-v "$(PWD)/go-webrtc-server:/build/go-webrtc-server/go-webrtc-server" \
		-v "$(PWD)/supercollider:/app/supercollider" \
		-v "$(PWD)/scripts:/app/scripts" \
		-v "$(PWD)/client:/app/client" \
		-v "/dev/snd:/dev/snd" \
		-v "/dev/shm:/dev/shm" \
		-e DEBIAN_FRONTEND=noninteractive \
		-e PION_LOG_TRACE=all \
		$(IMAGE_NAME)

