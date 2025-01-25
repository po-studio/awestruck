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
          // - allows stun/turn control traffic from clients
          // - enables nat traversal via turn
          // - required for webrtc ice connectivity
          fromPort: 3478,
          toPort: 3478,
          protocol: "udp",
          cidrBlocks: ["0.0.0.0/0"],
        },
        {
          // why we need ephemeral ports:
          // - allows dynamic port allocation for webrtc media
          // - follows IANA standard port allocation (49152-65535)
          // - ensures consistent port range across services
          fromPort: 49152,
          toPort: 65535,
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

    const awestruckTargetGroup = new LbTargetGroup(this, "awestruck-tg", {
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
    
    // why we need a network load balancer for webrtc:
    // - handles udp traffic for media streams
    // - provides stable networking for webrtc
    // - enables proper port forwarding
    const webrtcNlb = new Lb(this, "awestruck-webrtc-nlb", {
      name: "awestruck-webrtc-nlb",
      internal: false,
      loadBalancerType: "network",
      subnets: [subnet1.id, subnet2.id],
      enableCrossZoneLoadBalancing: true,
    });
    
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

    // why we need to store task definition in a variable:
    // - enables reuse in multiple services
    // - allows referencing in other resources
    // - improves code maintainability
    const webrtcTaskDefinition = new EcsTaskDefinition(
      this,
      "awestruck-webrtc-task-definition",
      {
        family: "server-arm64",
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
              { containerPort: 8080, hostPort: 8080, protocol: "tcp" }
            ],
            environment: [
              { name: "DEPLOYMENT_TIMESTAMP", value: new Date().toISOString() },
              { name: "AWESTRUCK_ENV", value: "production" },
              { name: "JACK_NO_AUDIO_RESERVATION", value: "1" },
              { name: "JACK_RATE", value: "48000" },
              { name: "JACK_PERIOD_SIZE", value: "1024" },
              { name: "JACK_WAIT_TIME", value: "21333" },
              { name: "JACK_PLAYBACK_PORTS", value: "2" },
              { name: "JACK_CAPTURE_PORTS", value: "2" },
              { name: "OPENAI_API_KEY", value: "{{resolve:ssm:/awestruck/openai_api_key:1}}" },
              { name: "AWESTRUCK_API_KEY", value: "{{resolve:ssm:/awestruck/awestruck_api_key:1}}" },
              // why we need turn server dns:
              // - enables service discovery in aws
              // - matches turn server's alias record
              // - ensures consistent networking
              { name: "TURN_SERVER", value: "turn.awestruck.io" }
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

    const listener = new LbListener(this, "awestruck-https-listener", {
      loadBalancerArn: alb.arn,
      port: 443,
      protocol: "HTTPS",
      sslPolicy: "ELBSecurityPolicy-2016-08",
      certificateArn: sslCertificateArn,
      defaultAction: [
        {
          type: "forward",
          targetGroupArn: awestruckTargetGroup.arn,
        },
      ],
    });

    // why we need a turn target group:
    // - handles stun/turn control traffic on port 3478
    // - enables health checks for turn service
    // - routes turn traffic through fargate tasks
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

    // why we need a turn listener:
    // - forwards stun/turn traffic to turn service
    // - enables nat traversal via turn
    // - maintains persistent turn connections
    const turnListener = new LbListener(this, "turn-udp-listener", {
      loadBalancerArn: webrtcNlb.arn,
      port: 3478,
      protocol: "UDP",
      defaultAction: [{
        type: "forward",
        targetGroupArn: turnTargetGroup.arn
      }]
    });

    // why we need a dedicated webrtc service:
    // - separates concerns from other services
    // - enables independent scaling
    // - simplifies monitoring and maintenance
    new EcsService(this, "webrtc-service", {
      name: "webrtc-service",
      cluster: ecsCluster.arn,
      taskDefinition: webrtcTaskDefinition.arn,
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
          targetGroupArn: awestruckTargetGroup.arn,
          containerName: "server-arm64",
          containerPort: 8080,
        }
      ],
      dependsOn: [listener],
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
              { containerPort: 3478, hostPort: 3478, protocol: "udp" },
              { containerPort: 3479, hostPort: 3479, protocol: "tcp" }
            ],
            environment: [
              { name: "AWESTRUCK_ENV", value: "production" },
              { name: "SIGNALING_PORT", value: "3478" },
              { name: "HEALTH_PORT", value: "3479" },
              // why we need correct realm:
              // - matches main domain for authentication
              // - ensures consistent ice credentials
              // - required for webrtc security
              { name: "TURN_REALM", value: "awestruck.io" },
              { name: "EXTERNAL_IP", value: "turn.awestruck.io" },
              { name: "TURN_USERNAME", value: "dummy-username" },
              { name: "TURN_PASSWORD", value: "c07f1c92982b4621af1d1a63dda5539c" }
            ],
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

    // why we need to expose the nlb dns name:
    // - helps with debugging turn connectivity
    // - enables direct nlb access if needed
    // - supports dns-based failover
    new TerraformOutput(this, "turn-nlb-dns", {
      value: webrtcNlb.dnsName,
      description: "Network Load Balancer DNS name for TURN server",
    });

    // Update turn-service to use the main security group
    new EcsService(this, "turn-service", {
      name: "awestruck-turn-service",
      cluster: ecsCluster.arn,
      taskDefinition: turnTaskDefinition.arn,
      desiredCount: 1,
      launchType: "FARGATE",
      forceNewDeployment: true,
      networkConfiguration: {
        assignPublicIp: true,
        subnets: [subnet1.id, subnet2.id],
        securityGroups: [securityGroup.id],  // Use main security group
      },
      loadBalancer: [{
        targetGroupArn: turnTargetGroup.arn,
        containerName: "turn-server",
        containerPort: 3478
      }],
      dependsOn: [turnListener]
    });

    // Add CloudWatch dashboard for monitoring both services
    new CloudwatchDashboard(this, "awestruck-dashboard", {
      dashboardName: "awestruck-services",
      dashboardBody: JSON.stringify({
        widgets: [
          // why we need connection failure analysis:
          // - track exact timing of failures
          // - identify failure patterns
          // - correlate with ice/dtls events
          {
            type: "log",
            x: 0,
            y: 0,
            width: 24,
            height: 6,
            properties: {
              query: `SOURCE '${webrtcLogGroup.name}' | 
                filter @timestamp > ago(1h) |
                filter @message like /\\[ICE\\]|\\[WebRTC\\]|\\[DTLS\\]/ |
                sort @timestamp asc |
                display @timestamp, @message`,
              region: awsRegion,
              title: "Connection Event Timeline",
              view: "table"
            },
          },
          // why we need ice candidate analysis:
          // - verify relay candidates are generated
          // - check candidate gathering process
          // - identify networking issues
          {
            type: "log",
            x: 0,
            y: 6,
            width: 12,
            height: 6,
            properties: {
              query: `SOURCE '${webrtcLogGroup.name}' | 
                filter @timestamp > ago(1h) |
                filter @message like /Processing candidate/ |
                parse @message "*] Processing candidate: protocol=* address=* port=* priority=* type=*" as prefix, protocol, address, port, priority, type |
                stats count(*) as count by type, protocol |
                sort count desc`,
              region: awsRegion,
              title: "ICE Candidate Distribution",
              view: "table"
            },
          },
          // why we need audio pipeline tracing:
          // - track audio flow from source to sink
          // - identify where audio stops
          // - debug pipeline configuration
          {
            type: "log",
            x: 12,
            y: 6,
            width: 12,
            height: 6,
            properties: {
              query: `SOURCE '${webrtcLogGroup.name}' | 
                filter @timestamp > ago(1h) |
                filter @message like /\\[AUDIO\\]|\\[GST\\]|Pipeline|JACK/ |
                sort @timestamp asc |
                display @timestamp, @message`,
              region: awsRegion,
              title: "Audio Pipeline Events",
              view: "table"
            },
          },
          // why we need turn server diagnostics:
          // - verify turn authentication
          // - track relay allocation
          // - monitor turn connectivity
          {
            type: "log",
            x: 0,
            y: 12,
            width: 12,
            height: 6,
            properties: {
              query: `SOURCE '${turnLogGroup.name}' | 
                filter @timestamp > ago(1h) |
                filter @message like /Authentication|allocation|ERROR/ |
                sort @timestamp asc |
                display @timestamp, @message`,
              region: awsRegion,
              title: "TURN Server Events",
              view: "table"
            },
          },
          // why we need session correlation:
          // - track individual session lifecycle
          // - correlate events across components
          // - identify session-specific issues
          {
            type: "log",
            x: 12,
            y: 12,
            width: 12,
            height: 6,
            properties: {
              query: `SOURCE '${webrtcLogGroup.name}' | 
                filter @timestamp > ago(1h) |
                parse @message "*sid_*_*]*" as prefix, session_id, suffix |
                stats count(*) as event_count by session_id |
                sort event_count desc |
                limit 10`,
              region: awsRegion,
              title: "Session Activity",
              view: "table"
            },
          },
          // why we need error correlation:
          // - identify cascading failures
          // - track error sequences
          // - find root causes
          {
            type: "log",
            x: 0,
            y: 18,
            width: 24,
            height: 6,
            properties: {
              query: `SOURCE '${webrtcLogGroup.name}' | 
                filter @timestamp > ago(1h) |
                filter @message like /ERROR|WARN|failed|disconnected/ |
                sort @timestamp asc |
                display @timestamp, @message`,
              region: awsRegion,
              title: "Error Timeline",
              view: "table"
            },
          }
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