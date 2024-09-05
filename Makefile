IMAGE_NAME=go-webrtc-server-arm64
CONTAINER_NAME=go-webrtc-server-arm64-instance
BROWSER_PORT=8080

.PHONY: build up down rshell shell open-browser

build:
	docker buildx build --platform linux/arm64 -t $(IMAGE_NAME) .

up: 
	docker run --rm --name $(CONTAINER_NAME) \
		--platform linux/arm64 \
		-p $(BROWSER_PORT):$(BROWSER_PORT) \
		--network host \
		--shm-size=1g \
		--ulimit memlock=-1 \
		--ulimit stack=67108864 \
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