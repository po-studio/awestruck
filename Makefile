# Makefile for building and running a Docker container based on your Docker Compose configuration

IMAGE_NAME=go-webrtc-server
CONTAINER_NAME=go-webrtc-server-instance
BROWSER_PORT=8080

.PHONY: build run open-browser

build:
	docker build -t $(IMAGE_NAME) .

up: 
	docker run --rm --name $(CONTAINER_NAME) \
		-p $(BROWSER_PORT):$(BROWSER_PORT) \
		$(IMAGE_NAME)
		
down:
	@echo "Stopping the container..."
	-docker stop $(CONTAINER_NAME)
	@echo "Stopping SuperCollider server..."
	-docker exec -it $(CONTAINER_NAME) pkill scsynth || true
	@echo "Everything stopped gracefully."

rshell:
	@CONTAINER_ID=$$(docker ps --filter "ancestor=go-webrtc-server" --format "{{.ID}}" | head -n 1); \
	if [ -z "$$CONTAINER_ID" ]; then \
		echo "No running container found for 'go-webrtc-server'."; \
	else \
		docker exec -u root -it $$CONTAINER_ID /bin/bash; \
	fi

shell:
	@CONTAINER_ID=$$(docker ps --filter "ancestor=go-webrtc-server" --format "{{.ID}}" | head -n 1); \
	if [ -z "$$CONTAINER_ID" ]; then \
		echo "No running container found for 'go-webrtc-server'."; \
	else \
		docker exec -it $$CONTAINER_ID /bin/bash; \
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