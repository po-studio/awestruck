import { Construct } from "constructs";
import { App, TerraformStack } from "cdktf";
import { AwsProvider } from "@cdktf/provider-aws/lib/provider";
import { Vpc } from "@cdktf/provider-aws/lib/vpc";
import { InternetGateway } from "@cdktf/provider-aws/lib/internet-gateway";
import { Subnet } from "@cdktf/provider-aws/lib/subnet";
import { RouteTable } from "@cdktf/provider-aws/lib/route-table";
import { Route } from "@cdktf/provider-aws/lib/route";
import { RouteTableAssociation } from "@cdktf/provider-aws/lib/route-table-association";
import { SecurityGroup } from "@cdktf/provider-aws/lib/security-group";
import { Lb as AlbLoadBalancer } from "@cdktf/provider-aws/lib/lb";
import { LbTargetGroup } from "@cdktf/provider-aws/lib/lb-target-group";
import { LbListener } from "@cdktf/provider-aws/lib/lb-listener";
import { EcsCluster } from "@cdktf/provider-aws/lib/ecs-cluster";
import { EcsService } from "@cdktf/provider-aws/lib/ecs-service";
import { EcsTaskDefinition } from "@cdktf/provider-aws/lib/ecs-task-definition";
import { IamRole } from "@cdktf/provider-aws/lib/iam-role";
import { IamRolePolicyAttachment } from "@cdktf/provider-aws/lib/iam-role-policy-attachment";
import { CloudwatchLogGroup } from "@cdktf/provider-aws/lib/cloudwatch-log-group";
import { DataAwsRoute53Zone } from "@cdktf/provider-aws/lib/data-aws-route53-zone";
import { Route53Record } from "@cdktf/provider-aws/lib/route53-record";
import { Instance } from "@cdktf/provider-aws/lib/instance";
import { SecurityGroupRule } from "@cdktf/provider-aws/lib/security-group-rule";
import { TerraformOutput } from "cdktf";
import * as dotenv from 'dotenv';
import { SsmParameter } from "@cdktf/provider-aws/lib/ssm-parameter";
import { IamInstanceProfile } from "@cdktf/provider-aws/lib/iam-instance-profile";

dotenv.config();

class AwestruckInfrastructure extends TerraformStack {
  constructor(scope: Construct, id: string) {
    super(scope, id);

    const awsAccountId = process.env.AWS_ACCOUNT_ID || this.node.tryGetContext('awsAccountId');
    const sslCertificateArn = process.env.SSL_CERTIFICATE_ARN || this.node.tryGetContext('sslCertificateArn');
    const turnPassword = process.env.TURN_PASSWORD || this.node.tryGetContext('turnPassword');
    
    const letsEncryptEmail = process.env.LETS_ENCRYPT_EMAIL || this.node.tryGetContext('letsEncryptEmail');
    if (!awsAccountId || !sslCertificateArn || !turnPassword || !letsEncryptEmail) {
      throw new Error('AWS_ACCOUNT_ID, SSL_CERTIFICATE_ARN, TURN_PASSWORD, and LETS_ENCRYPT_EMAIL must be set in environment variables or cdktf.json context');
    }

    new AwsProvider(this, "AWS", {
      region: this.node.tryGetContext("awsRegion") || "us-east-1",
    });

    const vpc = new Vpc(this, "awestruck-vpc", {
      cidrBlock: "10.0.0.0/16",
      enableDnsHostnames: true,
      tags: {
        Name: "awestruck-vpc",
      },
    });

    const internetGateway = new InternetGateway(this, "awestruck-igw", {
      vpcId: vpc.id,
      tags: {
        Name: "awestruck-igw",
      },
    });

    const subnet1 = new Subnet(this, "awestruck-subnet-1", {
      vpcId: vpc.id,
      cidrBlock: "10.0.1.0/24",
      availabilityZone: "us-east-1a",
      mapPublicIpOnLaunch: true,
      tags: {
        Name: "awestruck-subnet-1",
      },
    });

    const subnet2 = new Subnet(this, "awestruck-subnet-2", {
      vpcId: vpc.id,
      cidrBlock: "10.0.2.0/24",
      availabilityZone: "us-east-1b",
      mapPublicIpOnLaunch: true,
      tags: {
        Name: "awestruck-subnet-2",
      },
    });

    const routeTable = new RouteTable(this, "awestruck-route-table", {
      vpcId: vpc.id,
      tags: {
        Name: "awestruck-route-table",
      },
    });

    new Route(this, "internet-route", {
      routeTableId: routeTable.id,
      destinationCidrBlock: "0.0.0.0/0",
      gatewayId: internetGateway.id,
    });

    new RouteTableAssociation(this, "subnet1-route-table-association", {
      subnetId: subnet1.id,
      routeTableId: routeTable.id,
    });

    new RouteTableAssociation(this, "subnet2-route-table-association", {
      subnetId: subnet2.id,
      routeTableId: routeTable.id,
    });

    const securityGroup = new SecurityGroup(this, "awestruck-sg", {
      name: "awestruck-sg",
      description: "Security group for awestruck",
      vpcId: vpc.id,
      ingress: [
        {
          fromPort: 8080,
          toPort: 8080,
          protocol: "tcp",
          cidrBlocks: ["0.0.0.0/0"],
        },
        {
          fromPort: 443,
          toPort: 443,
          protocol: "tcp",
          cidrBlocks: ["0.0.0.0/0"],
        },
        {
          fromPort: 10000,
          toPort: 10010,
          protocol: "udp",
          cidrBlocks: ["0.0.0.0/0"],
        },
      ],
      egress: [
        {
          fromPort: 0,
          toPort: 0,
          protocol: "-1",
          cidrBlocks: ["0.0.0.0/0"],
        },
      ],
    });

    const targetGroup = new LbTargetGroup(this, "awestruck-tg", {
      name: "awestruck-tg-new",
      port: 8080,
      protocol: "HTTP",
      targetType: "ip",
      vpcId: vpc.id,
      healthCheck: {
        path: "/",
        interval: 30,
      },
    });

    const alb = new AlbLoadBalancer(this, "awestruck-alb", {
      name: "awestruck-alb-new",
      internal: false,
      loadBalancerType: "application",
      securityGroups: [securityGroup.id],
      subnets: [subnet1.id, subnet2.id],
    });

    const hostedZone = new DataAwsRoute53Zone(this, "hosted-zone", {
      name: "awestruck.io",
      privateZone: false,
    });

    new Route53Record(this, "awestruck-dns", {
      zoneId: hostedZone.zoneId,
      name: "awestruck.io",
      type: "A",
      allowOverwrite: true, // used for initial deployment
      lifecycle: {
        preventDestroy: true
      },
      alias: {
        name: alb.dnsName,
        zoneId: alb.zoneId,
        evaluateTargetHealth: true,
      },
    });

    const ecsCluster = new EcsCluster(this, "awestruck-cluster", {
      name: "awestruck",
    });

    const ecsTaskExecutionRole = new IamRole(this, "ecs-task-execution-role", {
      name: "awestruck-ecs-task-execution-role",
      assumeRolePolicy: JSON.stringify({
        Version: "2012-10-17",
        Statement: [
          {
            Effect: "Allow",
            Principal: {
              Service: "ecs-tasks.amazonaws.com",
            },
            Action: "sts:AssumeRole",
          },
        ],
      }),
    });

    new IamRolePolicyAttachment(this, "ecs-task-execution-role-policy", {
      role: ecsTaskExecutionRole.name,
      policyArn: "arn:aws:iam::aws:policy/service-role/AmazonECSTaskExecutionRolePolicy",
    });

    new IamRolePolicyAttachment(this, "cloudwatch-logs-full-access-policy", {
      role: ecsTaskExecutionRole.name,
      policyArn: "arn:aws:iam::aws:policy/CloudWatchLogsFullAccess",
    });

    const logGroup = new CloudwatchLogGroup(this, "awestruck-log-group", {
      name: `/ecs/${this.node.tryGetContext("taskDefinitionFamily")}`,
      retentionInDays: 30,
    });

    const ecsTaskRole = new IamRole(this, "ecs-task-role", {
      name: "awestruck-ecs-task-role",
      assumeRolePolicy: JSON.stringify({
        Version: "2012-10-17",
        Statement: [
          {
            Effect: "Allow",
            Principal: {
              Service: "ecs-tasks.amazonaws.com",
            },
            Action: "sts:AssumeRole",
          },
        ],
      }),
    });

    new IamRolePolicyAttachment(this, "ecs-task-ssm-policy", {
      role: ecsTaskRole.name,
      policyArn: "arn:aws:iam::aws:policy/AmazonSSMReadOnlyAccess",
    });

    const taskDefinition = new EcsTaskDefinition(this, "awestruck-task-definition", {
      family: "go-webrtc-server-arm64",
      cpu: "256",
      memory: "512",
      networkMode: "awsvpc",
      requiresCompatibilities: ["FARGATE"],
      executionRoleArn: ecsTaskExecutionRole.arn,
      taskRoleArn: ecsTaskRole.arn,
      runtimePlatform: {
        cpuArchitecture: "ARM64",
        operatingSystemFamily: "LINUX",
      },
      containerDefinitions: JSON.stringify([
        {
          name: "go-webrtc-server-arm64",
          image: `${awsAccountId}.dkr.ecr.${this.node.tryGetContext("awsRegion")}.amazonaws.com/po-studio/awestruck:latest`,
          portMappings: [
            { containerPort: 8080, hostPort: 8080, protocol: "tcp" },
            ...Array.from({ length: 11 }, (_, i) => ({
              containerPort: 10000 + i,
              hostPort: 10000 + i,
              protocol: "udp"
            }))
          ],
          environment: [
            { name: "DEPLOYMENT_TIMESTAMP", value: new Date().toISOString() },
            { name: "JACK_NO_AUDIO_RESERVATION", value: "1" },
            { name: "JACK_OPTIONS", value: "-R -d dummy" },
            { name: "JACK_SAMPLE_RATE", value: "48000" }
          ],
          ulimits: [
            { name: "memlock", softLimit: -1, hardLimit: -1 },
            { name: "stack", softLimit: 67108864, hardLimit: 67108864 }
          ],
          logConfiguration: {
            logDriver: "awslogs",
            options: {
              "awslogs-group": logGroup.name,
              "awslogs-region": this.node.tryGetContext("awsRegion"),
              "awslogs-stream-prefix": "ecs"
            }
          }
        }
      ])
    });

    const listener = new LbListener(this, "awestruck-https-listener", {
      loadBalancerArn: alb.arn,
      port: 443,
      protocol: "HTTPS",
      sslPolicy: "ELBSecurityPolicy-2016-08",
      certificateArn: sslCertificateArn,
      defaultAction: [
        {
          type: "forward",
          targetGroupArn: targetGroup.arn,
        },
      ],
    });

    new EcsService(this, "awestruck-service", {
      name: "awestruck-service",
      cluster: ecsCluster.arn,
      taskDefinition: taskDefinition.arn,
      desiredCount: 1,
      launchType: "FARGATE",
      forceNewDeployment: true,
      networkConfiguration: {
        assignPublicIp: true,
        subnets: [subnet1.id, subnet2.id],
        securityGroups: [securityGroup.id],
      },
      loadBalancer: [
        {
          targetGroupArn: targetGroup.arn,
          containerName: "go-webrtc-server-arm64",
          containerPort: 8080,
        },
      ],
      dependsOn: [listener],
    });

    const coturnSecurityGroup = new SecurityGroup(this, "coturn-security-group", {
      name: "coturn-security-group",
      vpcId: vpc.id,
      description: "Security group for COTURN server",
      tags: {
        Name: "coturn-security-group",
      },
    });

    // STUN/TURN ports
    const stunTurnPorts = [
      { port: 3478, protocol: "tcp" }, // STUN/TURN
      { port: 3478, protocol: "udp" }, // STUN/TURN
      { port: 5349, protocol: "tcp" }, // STUN/TURN over TLS
      { port: 5349, protocol: "udp" }, // STUN/TURN over DTLS
    ];

    // Allow TURN relay ports
    new SecurityGroupRule(this, "coturn-relay-range", {
      type: "ingress",
      fromPort: 10000,
      toPort: 10010,
      protocol: "udp",
      cidrBlocks: ["0.0.0.0/0"],
      securityGroupId: coturnSecurityGroup.id,
    });

    // Add STUN/TURN specific ports
    stunTurnPorts.forEach(({ port, protocol }) => {
      new SecurityGroupRule(this, `coturn-${protocol}-${port}`, {
        type: "ingress",
        fromPort: port,
        toPort: port,
        protocol,
        cidrBlocks: ["0.0.0.0/0"],
        securityGroupId: coturnSecurityGroup.id,
      });
    });

    // Allow all outbound traffic
    new SecurityGroupRule(this, "coturn-egress", {
      type: "egress",
      fromPort: 0,
      toPort: 0,
      protocol: "-1",
      cidrBlocks: ["0.0.0.0/0"],
      securityGroupId: coturnSecurityGroup.id,
    });

    const coturnRole = new IamRole(this, "coturn-role", {
      name: "awestruck-coturn-role",
      assumeRolePolicy: JSON.stringify({
        Version: "2012-10-17",
        Statement: [
          {
            Action: "sts:AssumeRole",
            Principal: {
              Service: "ec2.amazonaws.com"
            },
            Effect: "Allow"
          }
        ]
      }),
    });

    new IamRolePolicyAttachment(this, "coturn-acm-policy", {
      role: coturnRole.name,
      policyArn: "arn:aws:iam::aws:policy/AWSCertificateManagerReadOnly"
    });

    new IamRolePolicyAttachment(this, "coturn-ssm-policy", {
      role: coturnRole.name,
      policyArn: "arn:aws:iam::aws:policy/AmazonSSMReadOnlyAccess"
    });

    new IamRolePolicyAttachment(this, "coturn-cloudwatch-policy", {
      role: coturnRole.name,
      policyArn: "arn:aws:iam::aws:policy/CloudWatchLogsFullAccess"
    });

    const coturnInstanceProfile = new IamInstanceProfile(this, "coturn-instance-profile", {
      name: "awestruck-coturn-profile",
      role: coturnRole.name
    });

    // Create COTURN EC2 instance
    const coturnInstance = new Instance(this, "coturn-server", {
      ami: "ami-0ed83e7a78a23014e",  // Amazon Linux 2023 AMI 2023.6.20241121.0 arm64
      instanceType: "t4g.small",     // Using ARM instance for cost efficiency
      subnetId: subnet1.id,
      vpcSecurityGroupIds: [coturnSecurityGroup.id],
      associatePublicIpAddress: true,
      iamInstanceProfile: coturnInstanceProfile.name,
      tags: {
        Name: "coturn-server",
      },
      lifecycle: {
        createBeforeDestroy: true
      },
      userData: `#!/bin/bash
        yum update -y
        yum install -y coturn certbot

        mkdir -p /var/log/coturn
        chown turnserver:turnserver /var/log/coturn
        chmod 755 /var/log/coturn
        
        echo "COTURN server started $(date)" >> /var/log/coturn/turnserver.log
        
        # Get SSL certificate using certbot
        certbot certonly --standalone -d turn.awestruck.io --agree-tos --non-interactive --email ${letsEncryptEmail}

        # Link certificates for TURN server
        ln -s /etc/letsencrypt/live/turn.awestruck.io/fullchain.pem /etc/ssl/turn_server_cert.pem
        ln -s /etc/letsencrypt/live/turn.awestruck.io/privkey.pem /etc/ssl/turn_server_pkey.pem
        
        cat > /etc/turnserver.conf << EOL
        realm=turn.awestruck.io
        listening-port=3478
        tls-listening-port=5349
        total-quota=1000
        user-quota=100
        no-multicast-peers
        relay-threads=8
        min-port=10000
        max-port=10010
        stale-nonce=600
        # Static auth credentials
        user=awestruck:${turnPassword}
        # Enable trickle ICE support
        allow-loopback-peers
        mobility
        no-cli
        cert=/etc/ssl/turn_server_cert.pem
        pkey=/etc/ssl/turn_server_pkey.pem
        cipher-list="ECDHE-RSA-AES256-GCM-SHA512:DHE-RSA-AES256-GCM-SHA512:ECDHE-RSA-AES256-GCM-SHA384:DHE-RSA-AES256-GCM-SHA384"
        log-file=/var/log/coturn/turnserver.log
        verbose
        debug
        # New logging options
        log-binding
        log-allocations
        log-session-lifetime
        EOL

        # Configure log rotation
        cat > /etc/logrotate.d/coturn << EOL
        /var/log/coturn/turnserver.log {
            daily
            rotate 7
            compress
            delaycompress
            missingok
            notifempty
            create 644 turnserver turnserver
        }
        EOL

        systemctl enable coturn
        systemctl start coturn
        
        # Tail logs to CloudWatch
        yum install -y amazon-cloudwatch-agent
        mkdir -p /opt/aws/amazon-cloudwatch-agent/etc
        cat > /opt/aws/amazon-cloudwatch-agent/etc/amazon-cloudwatch-agent.json << 'EOL'
        {
          "logs": {
            "logs_collected": {
              "files": {
                "collect_list": [
                  {
                    "file_path": "/var/log/coturn/turnserver.log",
                    "log_group_name": "/coturn/turnserver",
                    "log_stream_name": "{instance_id}",
                    "timezone": "UTC"
                  }
                ]
              }
            }
          }
        }
        EOL

        # Start CloudWatch agent
        /opt/aws/amazon-cloudwatch-agent/bin/amazon-cloudwatch-agent-ctl -a fetch-config -m ec2 -s -c file:/opt/aws/amazon-cloudwatch-agent/etc/amazon-cloudwatch-agent.json
        systemctl enable amazon-cloudwatch-agent
        systemctl start amazon-cloudwatch-agent
      `,
    });

    new Route53Record(this, "turn-dns", {
      zoneId: hostedZone.zoneId,
      name: "turn.awestruck.io",
      type: "A",
      ttl: 300,
      allowOverwrite: true, // used for initial deployment
      records: [coturnInstance.publicIp],
      lifecycle: {
        createBeforeDestroy: true
      },
      dependsOn: [coturnInstance]
    });

    // Add outputs for the TURN server details
    new TerraformOutput(this, "turn-server-details", {
      value: {
        domain: "turn.awestruck.io",
        username: "awestruck",
        ports: {
          stun: 3478,
          turn: 3478,
          turns: 5349
        }
      }
    });

    new SsmParameter(this, "turn-password", {
      name: "/awestruck/turn_password",
      type: "SecureString",
      value: turnPassword,
      description: "TURN server password for WebRTC connections"
    });

    new CloudwatchLogGroup(this, "coturn-logs", {
      name: "/coturn/turnserver",
      retentionInDays: 14,
      tags: {
        Name: "coturn-logs"
      }
    });

    new SsmParameter(this, "cloudwatch-agent-config", {
      name: "/AmazonCloudWatch-Config",
      type: "String",
      value: JSON.stringify({
        logs: {
          logs_collected: {
            files: {
              collect_list: [{
                file_path: "/var/log/coturn/turnserver.log",
                log_group_name: "/coturn/turnserver",
                log_stream_name: "{instance_id}",
                timezone: "UTC"
              }]
            }
          }
        }
      })
    });

    new IamRolePolicyAttachment(this, "ecs-task-cloudwatch-policy", {
      role: ecsTaskExecutionRole.name,
      policyArn: "arn:aws:iam::aws:policy/CloudWatchLogsFullAccess",
    });
  }
}

const app = new App();
new AwestruckInfrastructure(app, "infra");
app.synth();