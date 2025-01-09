#!/bin/bash
set -e
exec 1> >(logger -s -t $(basename $0)) 2>&1

# system updates and installation
yum update -y
amazon-linux-extras enable epel
yum install -y epel-release
yum install -y coturn amazon-cloudwatch-agent

# create required directories with proper permissions
mkdir -p /etc/coturn /var/log/coturn /run/coturn
chmod 755 /etc/coturn /var/log/coturn /run/coturn
chown turnserver:turnserver /run/coturn /var/log/coturn

# get instance metadata
LOCAL_IP=$(curl -s http://169.254.169.254/latest/meta-data/local-ipv4)
ELASTIC_IP=${ELASTIC_IP}

# configure coturn service directory
mkdir -p /etc/systemd/system/coturn.service.d/
cat > /etc/systemd/system/coturn.service.d/override.conf <<EOF
[Service]
User=turnserver
Group=turnserver
RuntimeDirectory=coturn
RuntimeDirectoryMode=0755
PIDFile=/run/coturn/turnserver.pid
EOF

# configure turn server
cat > /etc/coturn/turnserver.conf <<EOF
# network settings
listening-port=3478
listening-ip=\$LOCAL_IP
relay-ip=\$LOCAL_IP
external-ip=\$ELASTIC_IP
min-port=49152
max-port=65535

# authentication
lt-cred-mech
user=turnserver:${TURN_PASSWORD}
realm=awestruck.io

# logging configuration
log-file=/var/log/coturn/turnserver.log
syslog
log-binding
log-allocate
debug
extra-logging
trace

# security settings
no-multicast-peers
no-cli
mobility
fingerprint
cli-password=${TURN_PASSWORD}
total-quota=100
max-bps=0
no-auth-pings
no-tlsv1
no-tlsv1_1
stale-nonce=0
cipher-list="ECDHE-RSA-AES256-GCM-SHA512:DHE-RSA-AES256-GCM-SHA512:ECDHE-RSA-AES256-GCM-SHA384"
EOF

# set proper permissions for config
chown turnserver:turnserver /etc/coturn/turnserver.conf
chmod 644 /etc/coturn/turnserver.conf

# configure cloudwatch
cat > /opt/aws/amazon-cloudwatch-agent/etc/amazon-cloudwatch-agent.json <<EOF
{
    "agent": {
    "metrics_collection_interval": 60,
    "run_as_user": "root"
    },
    "logs": {
    "logs_collected": {
        "files": {
        "collect_list": [
            {
            "file_path": "/var/log/coturn/turnserver.log",
            "log_group_name": "/coturn/turnserver",
            "log_stream_name": "{instance_id}",
            "timezone": "UTC",
            "timestamp_format": "%Y-%m-%d %H:%M:%S",
            "multi_line_start_pattern": "^\\\\["
            },
            {
            "file_path": "/var/log/syslog",
            "log_group_name": "/coturn/system",
            "log_stream_name": "{instance_id}",
            "timezone": "UTC"
            }
        ]
        }
    },
    "force_flush_interval": 15
    }
}
EOF

# setup log file
touch /var/log/coturn/turnserver.log
chown turnserver:turnserver /var/log/coturn/turnserver.log
chmod 644 /var/log/coturn/turnserver.log

# start services
systemctl daemon-reload
systemctl enable amazon-cloudwatch-agent
systemctl start amazon-cloudwatch-agent
systemctl enable coturn
systemctl restart coturn