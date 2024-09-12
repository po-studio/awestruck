# Base image with common dependencies
FROM debian:buster AS base
ENV DEBIAN_FRONTEND=noninteractive
RUN apt-get update && apt-get install -y \
    jackd2 \
    gstreamer1.0-tools \
    gstreamer1.0-plugins-base \
    gstreamer1.0-plugins-good \
    gstreamer1.0-plugins-bad \
    gstreamer1.0-plugins-ugly \
    supercollider \
    libgstreamer1.0-dev \
    libgstreamer-plugins-base1.0-dev \
    libjack-jackd2-dev \
    libasound2-dev \
    procps \
    sudo \
    tcpdump \
    && rm -rf /var/lib/apt/lists/*

# Builder stage
FROM golang:1.18-buster AS builder
WORKDIR /go-webrtc-server

# Install GStreamer development packages
RUN apt-get update && apt-get install -y \
    libgstreamer1.0-dev \
    libgstreamer-plugins-base1.0-dev \
    gstreamer1.0-plugins-base \
    gstreamer1.0-plugins-good \
    gstreamer1.0-plugins-bad \
    gstreamer1.0-plugins-ugly \
    gstreamer1.0-libav

COPY go-webrtc-server/go.mod go-webrtc-server/go.sum ./
RUN go mod download
COPY go-webrtc-server/ .
RUN go build -o /app/webrtc-server .

# Final stage
FROM base AS final
RUN useradd -m appuser && \
    usermod -a -G audio appuser && \
    mkdir -p /tmp/runtime-appuser && \
    chown appuser:appuser /tmp/runtime-appuser

WORKDIR /app
RUN chown -R appuser:appuser /app && \
    echo "appuser ALL=(ALL) NOPASSWD:ALL" >> /etc/sudoers


COPY --from=builder /app/webrtc-server /app/webrtc-server
COPY supercollider /app/supercollider
COPY client /app/client

COPY startup.sh /app/startup.sh
RUN chmod +x /app/startup.sh

USER appuser

EXPOSE 8080

ENV GST_DEBUG=3 \
    JACK_NO_AUDIO_RESERVATION=1 \
    JACK_NO_START_SERVER=1 \
    XDG_RUNTIME_DIR=/tmp/runtime-appuser \
    JACK_SAMPLE_RATE=48000

CMD ["./startup.sh"]