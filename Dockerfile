FROM golang:1.18-buster as builder

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
    && rm -rf /var/lib/apt/lists/*

WORKDIR /go-webrtc-server
COPY go-webrtc-server/go.mod go-webrtc-server/go.sum ./
RUN go mod download
COPY go-webrtc-server/ .

RUN go build -o /app/webrtc-server .

FROM debian:buster

ENV DEBIAN_FRONTEND=noninteractive

RUN useradd -m appuser
RUN usermod -a -G audio appuser

ENV XDG_RUNTIME_DIR=/tmp/runtime-appuser
RUN mkdir -p $XDG_RUNTIME_DIR && chown appuser:appuser $XDG_RUNTIME_DIR

WORKDIR /app

RUN chown -R appuser:appuser /app

RUN apt-get update && apt-get install -y \
    procps \
    sudo \
    tcpdump \
    jackd2 \
    gstreamer1.0-plugins-base \
    gstreamer1.0-plugins-good \
    gstreamer1.0-tools \
    supercollider \
    libgstreamer1.0-dev \
    libgstreamer-plugins-base1.0-dev \
    && rm -rf /var/lib/apt/lists/*

# DANGER: for debugging purposes only
# Grant appuser sudo privileges without password prompt
RUN echo "appuser ALL=(ALL) NOPASSWD:ALL" >> /etc/sudoers

COPY startup.sh /app/startup.sh

RUN chmod +x /app/startup.sh

COPY --from=builder /app/webrtc-server /app/webrtc-server
COPY supercollider /app/supercollider
COPY client /app/client

USER appuser

EXPOSE 8080

CMD ["./startup.sh"]
