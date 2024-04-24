# Makefile for building and running a Docker container based on your Docker Compose configuration

IMAGE_NAME=go-webrtc-server
CONTAINER_NAME=go-webrtc-server-instance
BROWSER_PORT=8080

.PHONY: build run open-browser

build:
	docker build -t $(IMAGE_NAME) .

up: build
	docker run --rm --name $(CONTAINER_NAME) \
		--user "1000" \
		--privileged \
		--ulimit memlock=-1:-1 \
		--ulimit rtprio=99:99 \
		--cap-add SYS_NICE \
		-p $(BROWSER_PORT):$(BROWSER_PORT) \
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
		$(IMAGE_NAME) & echo "Attempting to open the browser..." & sleep 2 && $(MAKE) open-browser

down:
	@echo "Stopping the container..."
	-docker stop $(CONTAINER_NAME)
	@echo "Stopping SuperCollider server..."
	-docker exec -it $(CONTAINER_NAME) pkill sclang || true
	@echo "Everything stopped gracefully."


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