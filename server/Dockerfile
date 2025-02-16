# Base image with common dependencies
FROM debian:buster AS deps

ENV DEBIAN_FRONTEND=noninteractive

# Install all system dependencies in a single layer
RUN apt-get update && apt-get install -y \
    jackd2 \
    gstreamer1.0-tools \
    gstreamer1.0-plugins-base \
    gstreamer1.0-plugins-good \
    gstreamer1.0-plugins-bad \
    gstreamer1.0-plugins-ugly \
    supercollider-server \
    supercollider-language \
    libgstreamer1.0-dev \
    libgstreamer-plugins-base1.0-dev \
    libjack-jackd2-dev \
    libasound2-dev \
    procps \
    sudo \
    tcpdump \
    && rm -rf /var/lib/apt/lists/* \
    && echo "* soft memlock 524288" >> /etc/security/limits.conf \
    && echo "* hard memlock 524288" >> /etc/security/limits.conf \
    && echo "* soft rtprio 99" >> /etc/security/limits.conf \
    && echo "* hard rtprio 99" >> /etc/security/limits.conf

# Go builder stage with dependencies
FROM golang:1.18-buster AS builder

# Install system dependencies first (rarely changes)
RUN apt-get update && apt-get install -y \
    libgstreamer1.0-dev \
    libgstreamer-plugins-base1.0-dev \
    gstreamer1.0-plugins-base \
    gstreamer1.0-plugins-good \
    gstreamer1.0-plugins-bad \
    gstreamer1.0-plugins-ugly \
    gstreamer1.0-libav \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /server

# Copy only go.mod and go.sum first (change less frequently)
COPY go.mod go.sum ./
RUN go mod download

# Copy source code (changes most frequently)
COPY . .
RUN go build -o /app/webrtc-server .

# Final stage
FROM deps AS final

# Create user and setup permissions
RUN useradd -m appuser && \
    usermod -a -G audio appuser && \
    mkdir -p /tmp/runtime-appuser && \
    chown appuser:appuser /tmp/runtime-appuser

WORKDIR /app
# RUN chown -R appuser:appuser /app && \
#     echo "appuser ALL=(ALL) NOPASSWD:ALL" >> /etc/sudoers

# Copy built artifacts and application files
COPY --from=builder /app/webrtc-server /app/webrtc-server
COPY sc /app/sc
# COPY client /app/client
COPY startup.sh /app/startup.sh
# RUN chmod +x /app/startup.sh

RUN chown -R appuser:appuser /app && \
    chmod +x /app/startup.sh && \
    echo "appuser ALL=(ALL) NOPASSWD:ALL" >> /etc/sudoers

USER appuser

# Web/Signaling
EXPOSE 8080

ENV GST_DEBUG=4 \
    JACK_NO_AUDIO_RESERVATION=1 \
    JACK_NO_START_SERVER=1 \
    XDG_RUNTIME_DIR=/tmp/runtime-appuser \
    JACK_SAMPLE_RATE=48000 \
    AWESTRUCK_ENV=development

CMD ["./startup.sh"]