import { App } from "cdktf";
import { Construct } from "constructs";
import { TerraformStack, TerraformOutput } from "cdktf";
import { AwsProvider } from "@cdktf/provider-aws/lib/provider";
import { Vpc } from "@cdktf/provider-aws/lib/vpc";
import { SecurityGroup } from "@cdktf/provider-aws/lib/security-group";
import { Route53Record } from "@cdktf/provider-aws/lib/route53-record";
import { DataAwsRoute53Zone } from "@cdktf/provider-aws/lib/data-aws-route53-zone";
import { EcsCluster } from "@cdktf/provider-aws/lib/ecs-cluster";
import { EcsService } from "@cdktf/provider-aws/lib/ecs-service";
import { EcsTaskDefinition } from "@cdktf/provider-aws/lib/ecs-task-definition";
import { IamRole } from "@cdktf/provider-aws/lib/iam-role";
import { IamRolePolicyAttachment } from "@cdktf/provider-aws/lib/iam-role-policy-attachment";
import { Subnet } from "@cdktf/provider-aws/lib/subnet";
import { InternetGateway } from "@cdktf/provider-aws/lib/internet-gateway";
import { RouteTable } from "@cdktf/provider-aws/lib/route-table";
import { RouteTableAssociation } from "@cdktf/provider-aws/lib/route-table-association";
import { Route } from "@cdktf/provider-aws/lib/route";
import { Lb } from "@cdktf/provider-aws/lib/lb";
import { LbTargetGroup } from "@cdktf/provider-aws/lib/lb-target-group";
import { LbListener } from "@cdktf/provider-aws/lib/lb-listener";
import { CloudwatchLogGroup } from "@cdktf/provider-aws/lib/cloudwatch-log-group";
import { SsmParameter } from "@cdktf/provider-aws/lib/ssm-parameter";
import { CloudwatchDashboard } from "@cdktf/provider-aws/lib/cloudwatch-dashboard";
import * as dotenv from "dotenv";

dotenv.config();

class AwestruckInfrastructure extends TerraformStack {
  constructor(scope: Construct, id: string) {
    super(scope, id);

    const awsAccountId =
      process.env.AWS_ACCOUNT_ID || this.node.tryGetContext("awsAccountId");
    const sslCertificateArn =
      process.env.SSL_CERTIFICATE_ARN ||
      this.node.tryGetContext("sslCertificateArn");
    const awsRegion = this.node.tryGetContext("awsRegion") || "us-east-1";

    if (!awsAccountId || !sslCertificateArn) {
      throw new Error(
        "AWS_ACCOUNT_ID and SSL_CERTIFICATE_ARN must be set in environment variables or cdktf.json context"
      );
    }

    new AwsProvider(this, "AWS", {
      region: awsRegion,
    });

    const vpc = new Vpc(this, "awestruck-vpc", {
      cidrBlock: "10.0.0.0/16",
      enableDnsHostnames: true,
      enableDnsSupport: true,
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
          // why we need turn control port:
          // - allows stun/turn control traffic (3478/udp)
          // - enables nat traversal via turn
          // - required for webrtc ice connectivity
          fromPort: 3478,
          toPort: 3478,
          protocol: "udp",
          cidrBlocks: ["0.0.0.0/0"],
        },
        {
          // why we need internal vpc traffic:
          // - allows nlb health checks
          // - enables turn server communication
          // - required for service discovery
          fromPort: 0,
          toPort: 0,
          protocol: "-1",
          cidrBlocks: [vpc.cidrBlock],
        }
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
        enabled: true,
        path: "/",
        port: "8080",
        protocol: "HTTP",
        healthyThreshold: 2,
        unhealthyThreshold: 3,
        interval: 5,
        timeout: 2,
        matcher: "200-299"
      },
    });

    const alb = new Lb(this, "awestruck-alb", {
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
      allowOverwrite: true,
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
            Action: "sts:AssumeRole",
            Effect: "Allow",
            Principal: {
              Service: "ecs-tasks.amazonaws.com",
            },
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

    const webrtcLogGroup = new CloudwatchLogGroup(this, "webrtc-log-group", {
      name: `/ecs/webrtc-server`,
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

    new SsmParameter(this, "openai-api-key", {
      name: "/awestruck/openai_api_key",
      type: "SecureString",
      value: process.env.OPENAI_API_KEY || this.node.tryGetContext("openaiApiKey"),
      description: "OpenAI API key for AI services",
    });

    new SsmParameter(this, "awestruck-api-key", {
      name: "/awestruck/awestruck_api_key",
      type: "SecureString",
      value: process.env.AWESTRUCK_API_KEY || this.node.tryGetContext("awestruckApiKey"),
      description: "Awestruck API key for authentication",
    });

    new EcsTaskDefinition(
      this,
      "awestruck-webrtc-task-definition",
      {
        family: "server-arm64",
        // why we need these resources:
        // - 2 vCPU (2048) for audio processing
        // - 4GB memory (4096) is the minimum valid for 2 vCPU
        // - follows fargate supported cpu/memory combinations
        cpu: "2048",
        memory: "4096",
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
            name: "server-arm64",
            image: `${awsAccountId}.dkr.ecr.${awsRegion}.amazonaws.com/po-studio/awestruck/services/webrtc:latest`,
            portMappings: [
              // why we map these ports:
              // - http port (8080) for web traffic and signaling
              // - webrtc media handled by turn server
              { containerPort: 8080, hostPort: 8080, protocol: "tcp" }
            ],
            environment: [
              { name: "DEPLOYMENT_TIMESTAMP", value: new Date().toISOString() },
              { name: "AWESTRUCK_ENV", value: "production" },
              // why we need these jack settings:
              // - matches dummy driver's default configuration
              // - ensures consistent audio buffering
              // - maintains stability in non-realtime environments
              { name: "JACK_NO_AUDIO_RESERVATION", value: "1" },
              { name: "JACK_RATE", value: "48000" },
              { name: "JACK_PERIOD_SIZE", value: "1024" },
              { name: "JACK_WAIT_TIME", value: "21333" },
              { name: "JACK_PLAYBACK_PORTS", value: "2" },
              { name: "JACK_CAPTURE_PORTS", value: "2" },
              // "secrets" ... adjust later
              { name: "OPENAI_API_KEY", value: "{{resolve:ssm:/awestruck/openai_api_key:1}}" },
              { name: "AWESTRUCK_API_KEY", value: "{{resolve:ssm:/awestruck/awestruck_api_key:1}}" }
            ],
            ulimits: [
              { name: "memlock", softLimit: -1, hardLimit: -1 },
              { name: "stack", softLimit: 67108864, hardLimit: 67108864 },
              { name: "rtprio", softLimit: 99, hardLimit: 99 }
            ],
            logConfiguration: {
              logDriver: "awslogs",
              options: {
                "awslogs-group": webrtcLogGroup.name,
                "awslogs-region": awsRegion,
                "awslogs-stream-prefix": "webrtc",
              },
            }
          }
        ]),
      }
    );

    new LbListener(this, "awestruck-https-listener", {
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

    // why we need a network load balancer for webrtc:
    // - handles udp traffic for media streams
    // - preserves client ip addresses
    // - enables proper nat traversal
    const webrtcNlb = new Lb(this, "awestruck-webrtc-nlb", {
      name: "awestruck-webrtc-nlb",
      internal: false,
      loadBalancerType: "network",
      subnets: [subnet1.id, subnet2.id],
      enableCrossZoneLoadBalancing: true,
    });

    // why we need a turn target group:
    // - handles stun/turn control traffic
    // - enables health checks
    // - routes traffic to turn containers
    const turnTargetGroup = new LbTargetGroup(this, "awestruck-turn-tg", {
      name: "awestruck-turn-tg",
      port: 3478,
      protocol: "UDP",
      targetType: "ip",
      vpcId: vpc.id,
      healthCheck: {
        enabled: true,
        port: "3479",
        protocol: "TCP",
        interval: 30,
        timeout: 10,
        healthyThreshold: 3,
        unhealthyThreshold: 5
      }
    });

    // why we need udp listeners:
    // - handles stun/turn signaling on 3478
    // - enables media relay on dynamic ports
    new LbListener(this, "turn-udp-listener", {
      loadBalancerArn: webrtcNlb.arn,
      port: 3478,
      protocol: "UDP",
      defaultAction: [{
        type: "forward",
        targetGroupArn: turnTargetGroup.arn
      }]
    });

    // why we need minimal security groups:
    // - only expose necessary ports
    // - separate control from media traffic
    // - maintain security best practices
    const turnSecurityGroup = new SecurityGroup(this, "turn-security-group", {
      name: "awestruck-turn-sg",
      description: "Security group for TURN server",
      vpcId: vpc.id,
      ingress: [
        {
          // why we need stun/turn port:
          // - enables ice/stun/turn signaling
          // - handles nat traversal setup
          fromPort: 3478,
          toPort: 3478,
          protocol: "udp",
          cidrBlocks: ["0.0.0.0/0"],
        },
        {
          // why we need health check port:
          // - enables load balancer health monitoring
          // - tcp for reliable checks
          fromPort: 3479,
          toPort: 3479,
          protocol: "tcp",
          cidrBlocks: ["0.0.0.0/0"],
        },
        {
          // why we need media port range:
          // - enables webrtc media relay
          // - standard ephemeral port range
          // - required for turn functionality
          fromPort: 49152,
          toPort: 65535,
          protocol: "udp",
          cidrBlocks: ["0.0.0.0/0"],
        }
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

    // why we need a turn log group:
    // - centralizes turn server logs
    // - enables log retention policies 
    const turnLogGroup = new CloudwatchLogGroup(this, "turn-log-group", {
      name: `/ecs/turn-server`,
      retentionInDays: 30,
    });

    // why we need a turn task definition:
    // - runs our pion turn implementation
    // - handles only connection establishment
    // - no media relay as audio goes through nlb
    const turnTaskDefinition = new EcsTaskDefinition(
      this,
      "awestruck-turn-task-definition",
      {
        family: "turn-server",
        cpu: "512",
        memory: "1024",
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
            name: "turn-server",
            image: `${awsAccountId}.dkr.ecr.${awsRegion}.amazonaws.com/po-studio/awestruck/services/turn:latest`,
            portMappings: [
              // why we need these ports:
              // - 3478/udp for stun/turn signaling
              // - 3479/tcp for health checks
              // - ephemeral ports for media relay
              { containerPort: 3478, hostPort: 3478, protocol: "udp" },
              { containerPort: 3479, hostPort: 3479, protocol: "tcp" }
            ],
            environment: [
              { name: "DEPLOYMENT_TIMESTAMP", value: new Date().toISOString() },
              { name: "TURN_REALM", value: "awestruck.io" },
              { name: "TURN_PORT", value: "3478" },
              { name: "HEALTH_PORT", value: "3479" },
              { name: "AWESTRUCK_ENV", value: "production" },
              // why we need turn server address:
              // - tells turn server its own external address
              // - used for ice candidate generation
              // - enables proper nat traversal
              // why we need nlb dns name:
              // - enables proper nat traversal
              // - handles ip changes automatically
              // - maintains stable endpoint for clients
              { name: "EXTERNAL_IP", value: webrtcNlb.dnsName }
            ],
            healthCheck: {
              command: ["CMD-SHELL", "curl -f http://localhost:3479/health || exit 1"],
              interval: 30,
              timeout: 5,
              retries: 3,
              startPeriod: 60
            },
            logConfiguration: {
              logDriver: "awslogs",
              options: {
                "awslogs-group": turnLogGroup.name,
                "awslogs-region": awsRegion,
                "awslogs-stream-prefix": "turn",
              },
            }
          }
        ]),
      }
    );

    // why we need dns records for turn:
    // - enables client discovery of turn services
    // - allows for future ip changes without client updates
    new Route53Record(this, "turn-dns", {
      zoneId: hostedZone.zoneId,
      name: "turn.awestruck.io",
      type: "A",
      allowOverwrite: true,
      alias: {
        name: webrtcNlb.dnsName,
        zoneId: webrtcNlb.zoneId,
        evaluateTargetHealth: true
      }
    });

    // why we need to expose the nlb dns name:
    // - helps with debugging turn connectivity
    // - enables direct nlb access if needed
    // - supports dns-based failover
    new TerraformOutput(this, "turn-nlb-dns", {
      value: webrtcNlb.dnsName,
      description: "Network Load Balancer DNS name for TURN server",
    });

    // why we need a turn service:
    // - runs turn server in fargate
    // - handles nat traversal and media relay
    // - auto scales with demand
    new EcsService(this, "turn-service", {
      name: "turn-service",
      cluster: ecsCluster.arn,
      taskDefinition: turnTaskDefinition.arn,
      desiredCount: 1,
      launchType: "FARGATE",
      forceNewDeployment: true,
      networkConfiguration: {
        assignPublicIp: true,
        subnets: [subnet1.id, subnet2.id],
        securityGroups: [turnSecurityGroup.id],
      },
      loadBalancer: [
        {
          targetGroupArn: turnTargetGroup.arn,
          containerName: "turn-server",
          containerPort: 3478,
        }
      ]
    });

    // Add CloudWatch dashboard for monitoring both services
    new CloudwatchDashboard(this, "awestruck-dashboard", {
      dashboardName: "awestruck-services",
      dashboardBody: JSON.stringify({
        widgets: [
          {
            type: "log",
            x: 0,
            y: 0,
            width: 12,
            height: 6,
            properties: {
              query: `SOURCE '${webrtcLogGroup.name}' | fields @timestamp, @message | sort @timestamp desc | limit 100`,
              region: awsRegion,
              title: "WebRTC Server Logs",
            },
          },
          {
            type: "log",
            x: 12,
            y: 0,
            width: 12,
            height: 6,
            properties: {
              query: `SOURCE '${turnLogGroup.name}' | fields @timestamp, @message | sort @timestamp desc | limit 100`,
              region: awsRegion,
              title: "TURN Server Logs",
            },
          },
          {
            type: "metric",
            x: 0,
            y: 6,
            width: 12,
            height: 6,
            properties: {
              metrics: [
                ["AWS/ECS", "CPUUtilization", "ServiceName", "turn-service", "ClusterName", "awestruck"],
                [".", "MemoryUtilization", ".", ".", ".", "."]
              ],
              region: awsRegion,
              title: "TURN Server Resources",
              period: 300,
              stat: "Average",
            },
          },
        ],
      }),
    });

    // Attach ECR read policy to allow pulling images
    new IamRolePolicyAttachment(this, "ecs-ecr-policy", {
      role: ecsTaskExecutionRole.name,
      policyArn: "arn:aws:iam::aws:policy/AmazonEC2ContainerRegistryReadOnly",
    });

    // Add CloudWatch metrics permissions to task role
    new IamRolePolicyAttachment(this, "cloudwatch-metrics-policy", {
      role: ecsTaskRole.name,
      policyArn: "arn:aws:iam::aws:policy/CloudWatchAgentServerPolicy",
    });
  }
}

const app = new App();
new AwestruckInfrastructure(app, "infra");
app.synth();