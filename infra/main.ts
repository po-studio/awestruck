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
import * as dotenv from "dotenv";
import { SsmParameter } from "@cdktf/provider-aws/lib/ssm-parameter";
import { IamInstanceProfile } from "@cdktf/provider-aws/lib/iam-instance-profile";

dotenv.config();

class AwestruckInfrastructure extends TerraformStack {
  constructor(scope: Construct, id: string) {
    super(scope, id);

    const awsAccountId =
      process.env.AWS_ACCOUNT_ID || this.node.tryGetContext("awsAccountId");
    const sslCertificateArn =
      process.env.SSL_CERTIFICATE_ARN ||
      this.node.tryGetContext("sslCertificateArn");
    const turnPassword =
      process.env.TURN_PASSWORD || this.node.tryGetContext("turnPassword");
    const awsRegion = this.node.tryGetContext("awsRegion") || "us-east-1";

    if (!awsAccountId || !sslCertificateArn || !turnPassword) {
      throw new Error(
        "AWS_ACCOUNT_ID, SSL_CERTIFICATE_ARN, and TURN_PASSWORD must be set in environment variables or cdktf.json context"
      );
    }

    const userData = `#!/bin/bash
    # Enable logging to CloudWatch
    exec 1> >(logger -s -t $(basename $0)) 2>&1
    
    log() {
        echo "[$(date '+%Y-%m-%d %H:%M:%S')] $1"
    }
    error_exit() {
        log "ERROR: $1"
        exit 1
    }
    
    log "Starting TURN server setup..."
    
    # Install required packages
    dnf update -y || error_exit "Failed to update system"
    dnf install -y docker amazon-cloudwatch-agent jq aws-cli || error_exit "Failed to install packages"
    
    # Start and enable Docker
    systemctl start docker || error_exit "Failed to start Docker"
    systemctl enable docker || error_exit "Failed to enable Docker"
    
    # Pull the coturn image before trying to run it
    docker pull coturn/coturn || error_exit "Failed to pull coturn image"
    
    # Create required directories with proper permissions
    mkdir -p /etc/coturn /var/log/coturn /etc/ssl || error_exit "Failed to create directories"
    chmod 755 /var/log/coturn
    
    # Get certificate from ACM (keeping your existing cert logic)
    log "Fetching certificate from ACM..."
    CERT_ARN="${sslCertificateArn}"
    aws acm get-certificate --certificate-arn $CERT_ARN --region ${awsRegion} > /tmp/cert.json
    
    jq -r '.Certificate' /tmp/cert.json > /etc/ssl/turn.awestruck.io.crt
    jq -r '.PrivateKey' /tmp/cert.json > /etc/ssl/turn.awestruck.io.key
    chmod 600 /etc/ssl/turn.awestruck.io.key
    
    # Configure TURN server
    log "Creating TURN server configuration..."
    cat > /etc/coturn/turnserver.conf << EOL
    realm=turn.awestruck.io
    listening-port=3478
    tls-listening-port=5349
    min-port=10000
    max-port=10010
    use-auth-secret
    static-auth-secret=${turnPassword}
    verbose
    debug
    log-file=stdout
    no-multicast-peers
    cert=/etc/ssl/turn.awestruck.io.crt
    pkey=/etc/ssl/turn.awestruck.io.key
    EOL
    
    # Start TURN server with error handling
    log "Starting TURN server..."
    if ! docker ps | grep -q coturn; then
        if docker ps -a | grep -q coturn; then
            log "Removing existing coturn container..."
            docker rm -f coturn
        fi
        
        docker run -d --name coturn \
          --restart unless-stopped \
          --network host \
          --log-driver=awslogs \
          --log-opt awslogs-group=/coturn/turnserver \
          --log-opt awslogs-region=${awsRegion} \
          --log-opt awslogs-stream=coturn-$(hostname) \
          -v /etc/coturn:/etc/coturn \
          -v /etc/ssl:/etc/ssl \
          coturn/coturn:latest turnserver -c /etc/coturn/turnserver.conf || error_exit "Failed to start coturn container"
            
        # Verify container is running
        sleep 5
        if ! docker ps | grep -q coturn; then
            error_exit "Coturn container failed to start. Logs: $(docker logs coturn 2>&1)"
        fi
    fi

    # Enable log streaming to CloudWatch
    docker logs -f coturn >> /var/log/coturn/turnserver.log 2>&1 &

    log "Verifying TURN server ports..."
    sleep 5
    if ! netstat -tuln | grep -q ':3478 '; then
        error_exit "TURN server not listening on port 3478"
    fi
    if ! netstat -tuln | grep -q ':5349 '; then
        error_exit "TURN server not listening on port 5349"
    fi

    log "COTURN server setup completed successfully"
    `;

    new AwsProvider(this, "AWS", {
      region: awsRegion,
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
      // lifecycle: {
      //   preventDestroy: true
      // },
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
      policyArn:
        "arn:aws:iam::aws:policy/service-role/AmazonECSTaskExecutionRolePolicy",
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

    const taskDefinition = new EcsTaskDefinition(
      this,
      "awestruck-task-definition",
      {
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
            image: `${awsAccountId}.dkr.ecr.${awsRegion}.amazonaws.com/po-studio/awestruck:latest`,
            portMappings: [
              { containerPort: 8080, hostPort: 8080, protocol: "tcp" },
              ...Array.from({ length: 11 }, (_, i) => ({
                containerPort: 10000 + i,
                hostPort: 10000 + i,
                protocol: "udp",
              })),
            ],
            environment: [
              { name: "DEPLOYMENT_TIMESTAMP", value: new Date().toISOString() },
              { name: "JACK_NO_AUDIO_RESERVATION", value: "1" },
              { name: "JACK_OPTIONS", value: "-R -d dummy" },
              { name: "JACK_SAMPLE_RATE", value: "48000" },
            ],
            ulimits: [
              { name: "memlock", softLimit: -1, hardLimit: -1 },
              { name: "stack", softLimit: 67108864, hardLimit: 67108864 },
            ],
            logConfiguration: {
              logDriver: "awslogs",
              options: {
                "awslogs-group": logGroup.name,
                "awslogs-region": awsRegion,
                "awslogs-stream-prefix": "ecs",
              },
            },
          },
        ]),
      }
    );

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

    const coturnSecurityGroup = new SecurityGroup(
      this,
      "coturn-security-group",
      {
        name: "coturn-security-group",
        vpcId: vpc.id,
        description: "Security group for COTURN server",
        tags: {
          Name: "coturn-security-group",
        },
      }
    );

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

    new SecurityGroupRule(this, "coturn-https-inbound", {
      type: "ingress",
      fromPort: 443,
      toPort: 443,
      protocol: "tcp",
      cidrBlocks: ["0.0.0.0/0"],
      securityGroupId: coturnSecurityGroup.id,
    });

    new SecurityGroupRule(this, "coturn-http-inbound", {
      type: "ingress",
      fromPort: 80,
      toPort: 80,
      protocol: "tcp",
      cidrBlocks: ["0.0.0.0/0"],
      securityGroupId: coturnSecurityGroup.id,
    });

    const coturnInstanceRole = new IamRole(this, "coturn-instance-role", {
      name: "coturn-instance-role",
      assumeRolePolicy: JSON.stringify({
        Version: "2012-10-17",
        Statement: [
          {
            Action: "sts:AssumeRole",
            Effect: "Allow",
            Principal: {
              Service: "ec2.amazonaws.com",
            },
          },
        ],
      }),
    });

    new IamRolePolicyAttachment(this, "coturn-ssm-policy", {
      role: coturnInstanceRole.name,
      policyArn: "arn:aws:iam::aws:policy/AmazonSSMManagedInstanceCore",
    });

    new IamRolePolicyAttachment(this, "coturn-acm-policy", {
      role: coturnInstanceRole.name,
      policyArn: "arn:aws:iam::aws:policy/AWSCertificateManagerReadOnly",
    });

    const coturnInstanceProfile = new IamInstanceProfile(
      this,
      "coturn-instance-profile",
      {
        name: "coturn-instance-profile",
        role: coturnInstanceRole.name,
      }
    );

    const coturnInstance = new Instance(this, "coturn-server", {
      ami: "ami-0ed83e7a78a23014e",
      instanceType: "t4g.small",
      subnetId: subnet1.id,
      vpcSecurityGroupIds: [coturnSecurityGroup.id],
      associatePublicIpAddress: true,
      iamInstanceProfile: coturnInstanceProfile.name,
      tags: {
        Name: "coturn-server",
      },
      lifecycle: {
        createBeforeDestroy: true,
      },
      userData: userData,
    });

    new Route53Record(this, "turn-dns", {
      zoneId: hostedZone.zoneId,
      name: "turn.awestruck.io",
      type: "A",
      ttl: 300,
      records: [coturnInstance.publicIp],
      allowOverwrite: true,
      lifecycle: {
        createBeforeDestroy: true,
      },
      dependsOn: [coturnInstance],
    });

    // Add outputs for the TURN server details
    new TerraformOutput(this, "turn-server-details", {
      value: {
        domain: "turn.awestruck.io",
        username: "awestruck",
        ports: {
          stun: 3478,
          turn: 3478,
          turns: 5349,
        },
      },
    });

    new SsmParameter(this, "turn-password", {
      name: "/awestruck/turn_password",
      type: "SecureString",
      value: turnPassword,
      description: "TURN server password for WebRTC connections",
    });

    new CloudwatchLogGroup(this, "coturn-logs", {
      name: "/coturn/turnserver",
      retentionInDays: 14,
      tags: {
        Name: "coturn-logs",
      },
    });

    new SsmParameter(this, "cloudwatch-agent-config", {
      name: "/AmazonCloudWatch-Config",
      type: "String",
      value: JSON.stringify({
        logs: {
          logs_collected: {
            files: {
              collect_list: [
                {
                  file_path: "/var/log/coturn/turnserver.log",
                  log_group_name: "/coturn/turnserver",
                  log_stream_name: "{instance_id}",
                  timezone: "UTC",
                },
              ],
            },
          },
        },
      }),
    });

    new IamRolePolicyAttachment(this, "ecs-task-cloudwatch-policy", {
      role: ecsTaskExecutionRole.name,
      policyArn: "arn:aws:iam::aws:policy/CloudWatchLogsFullAccess",
    });

    new IamRolePolicyAttachment(this, "coturn-cloudwatch-policy", {
      role: coturnInstanceRole.name,
      policyArn: "arn:aws:iam::aws:policy/CloudWatchLogsFullAccess",
    });
  }
}

const app = new App();
new AwestruckInfrastructure(app, "infra");
app.synth();
