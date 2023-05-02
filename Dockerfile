FROM --platform=linux/amd64 debian:latest

ENV DEBIAN_FRONTEND noninteractive

RUN apt-get update && apt-get install -y \
    libgstreamer1.0-0 \
    libgstreamer-plugins-base1.0-dev \
    gstreamer1.0-plugins-base \
    gstreamer1.0-plugins-good \
    gstreamer1.0-plugins-bad \
    gstreamer1.0-plugins-ugly \
    gstreamer1.0-libav \
    gstreamer1.0-tools \
    gstreamer1.0-x \
    gstreamer1.0-alsa \
    gstreamer1.0-gl \
    gstreamer1.0-gtk3 \
    gstreamer1.0-qt5 \
    git \
    apt-transport-https \
    ca-certificates \
    build-essential \
    wget \
    pkg-config \
    xvfb \
    xauth \
    supercollider \
    libcgroup-dev \
    alsa-utils

RUN wget https://dl.google.com/go/go1.13.linux-amd64.tar.gz
RUN tar -xvf go1.13.linux-amd64.tar.gz
RUN mv go /usr/local

ENV GOROOT /usr/local/go
ENV GOPATH $HOME/go
ENV PATH $GOPATH/bin:$GOROOT/bin:$PATH
ENV GO111MODULE=on

ENV PKG_CONFIG_PATH=/usr/local/opt/libffi/lib/pkgconfig

COPY startup.scd /root/.config/SuperCollider/startup.scd
COPY startup.scd ~/.config/SuperCollider/startup.scd

RUN dpkg-reconfigure -p high jackd

RUN apt-get update && apt-get install -y liboscpack-dev cmake
RUN git clone https://github.com/yoggy/sendosc.git
WORKDIR /sendosc
RUN cmake .
RUN make
RUN make install

WORKDIR /..

RUN apt-get update && apt-get install -y curl

RUN go get github.com/po-studio/example-webrtc-applications/v3@1b18c4594b648ef48c2da02cfd65e8617e4fc2d8
COPY webserver $GOPATH/pkg/mod/github.com/po-studio/awestruck/webserver
COPY go.mod go.mod
WORKDIR $GOPATH/pkg/mod/github.com/po-studio/awestruck/webserver

RUN go build -o /bin/awestruck main.go
WORKDIR /

COPY sccmd.sh /usr/src/sccmd.sh

RUN useradd -ms /bin/bash po
RUN usermod -a -G audio po
RUN chown -R po:po /usr/src/
RUN chown -R po:po ~/.config/
RUN chmod 755 /usr/src/
USER po
RUN chmod +x /usr/src/sccmd.sh

COPY asoundrc /root/.asoundrc
COPY asoundrc /usr/src/.asoundrc
COPY asoundrc /home/po/.asoundrc

COPY startup.scd ~/.config/SuperCollider/startup.scd
COPY startup.scd /home/po/.config/SuperCollider/startup.scd
COPY tst.sc /usr/src/tst.sc
COPY webserver webserver

EXPOSE 8080
ENTRYPOINT ["/bin/awestruck"]
