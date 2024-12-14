#!/bin/bash

mkdir -p certs
openssl req -x509 -newkey rsa:4096 -keyout certs/turn.localhost.key \
  -out certs/turn.localhost.crt -days 365 -nodes \
  -subj "/CN=turn.localhost"
chmod 644 certs/turn.localhost.crt
chmod 600 certs/turn.localhost.key