# Use a more recent Go base image
FROM golang:1.18-buster as builder

# RUN echo "jackd2 jackd/tweak_rt_limits boolean true" | debconf-set-selections

# Set non-interactive installation mode
ENV DEBIAN_FRONTEND=noninteractive
# ENV QT_QPA_PLATFORM=offscreen

# Install system dependencies
RUN apt-get update && apt-get install -y \
    jackd2 \
    gstreamer1.0-tools \
    gstreamer1.0-plugins-base \
    gstreamer1.0-plugins-good \
    gstreamer1.0-plugins-bad \
    gstreamer1.0-plugins-ugly \
    supercollider \
    xvfb \
    libgstreamer1.0-dev \
    libgstreamer-plugins-base1.0-dev \
    && rm -rf /var/lib/apt/lists/*

# Copy the Go application files into the container
WORKDIR /go-webrtc-server
COPY go-webrtc-server/go.mod go-webrtc-server/go.sum ./
RUN go mod download
COPY go-webrtc-server/ .

# Build the Go WebRTC server
RUN go build -o /app/webrtc-server .

# Start a new stage from scratch for a smaller, final image
FROM debian:buster

ENV DEBIAN_FRONTEND=noninteractive
# ENV QT_QPA_PLATFORM=offscreen

# Create a non-root user before attempting to use it in chown
RUN useradd -m appuser
RUN usermod -a -G audio appuser

ENV XDG_RUNTIME_DIR=/tmp/runtime-appuser
RUN mkdir -p $XDG_RUNTIME_DIR && chown appuser:appuser $XDG_RUNTIME_DIR

WORKDIR /app

# Now that appuser exists, you can safely change ownership
# No need for sudo, as Docker RUN commands execute as root by default
RUN chown -R appuser:appuser /app

# Install runtime dependencies
# Include gstreamer1.0-tools here
RUN apt-get update && apt-get install -y \
    jackd2 \
    gstreamer1.0-plugins-base \
    gstreamer1.0-plugins-good \
    gstreamer1.0-tools \
    supercollider \
    xvfb \
    libgstreamer1.0-dev \
    libgstreamer-plugins-base1.0-dev \
    && rm -rf /var/lib/apt/lists/*

COPY startup.scd /home/appuser/.config/SuperCollider/startup.scd

COPY --from=builder /app/webrtc-server /app/webrtc-server
COPY supercollider /app/supercollider

# Copy the client files
COPY client /app/client

# RUN dpkg-reconfigure -p high jackd

# Ensure to switch to the non-root user at the end of your Dockerfile
USER appuser

EXPOSE 8080

CMD jackd -r --port-max 20 -d dummy & /app/webrtc-server