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
import { Eip } from "@cdktf/provider-aws/lib/eip";
import * as dotenv from "dotenv";

const TURN_MIN_PORT = process.env.TURN_MIN_PORT ? parseInt(process.env.TURN_MIN_PORT) : 49152;
const TURN_MAX_PORT = process.env.TURN_MAX_PORT ? parseInt(process.env.TURN_MAX_PORT) : 49252;

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

    const subnet = new Subnet(this, "awestruck-subnet", {
      vpcId: vpc.id,
      cidrBlock: "10.0.1.0/24",
      availabilityZone: "us-east-1a",
      mapPublicIpOnLaunch: true,
      tags: {
        Name: "awestruck-subnet",
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

    new RouteTableAssociation(this, "subnet-route-table-association", {
      subnetId: subnet.id,
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
          description: "HTTPS for client application"
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
          // why we need health check port:
          // - enables nlb health checks via tcp
          // - ensures service availability monitoring
          // - required for target group registration
          fromPort: 3479,
          toPort: 3479,
          protocol: "tcp",
          cidrBlocks: [vpc.cidrBlock],
        },
        {
          // why we need ephemeral ports:
          // - allows dynamic port allocation for webrtc media
          // - matches docker-compose configuration
          // - ensures consistent port range across environments
          fromPort: TURN_MIN_PORT,
          toPort: TURN_MAX_PORT,
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
        },
        {
          fromPort: 5173,
          toPort: 5173,
          protocol: "tcp",
          cidrBlocks: ["0.0.0.0/0"],
          description: "Vite production server"
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

    const hostedZone = new DataAwsRoute53Zone(this, "hosted-zone", {
      name: "awestruck.io",
      privateZone: false,
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

    // why we need elastic ip for turn:
    // - provides stable ip for turn server
    // - survives nlb replacements
    // - enables consistent ice candidates
    const turnElasticIp = new Eip(this, "turn-eip", {
      vpc: true,
      tags: {
        Name: "turn-nlb-eip"
      }
    });

    // why we need a simplified nlb setup:
    // - reduces aws resource usage
    // - simplifies traffic flow
    // - maintains essential turn functionality
    const webrtcNlb = new Lb(this, "awestruck-webrtc-nlb", {
      name: "awestruck-webrtc-nlb",
      internal: false,
      loadBalancerType: "network",
      subnetMapping: [{
        subnetId: subnet.id,
        allocationId: turnElasticIp.allocationId
      }],
      enableCrossZoneLoadBalancing: false,
      ipAddressType: "ipv4",
      tags: {
        Name: "awestruck-webrtc-nlb"
      }
    });

    // why we need separate dns records:
    // - clearly separates service endpoints
    // - enables independent ssl certificates
    // - simplifies service discovery
    new Route53Record(this, "client-dns", {
      zoneId: hostedZone.id,
      name: "awestruck.io",
      type: "A",
      alias: {
        name: webrtcNlb.dnsName,
        zoneId: webrtcNlb.zoneId,
        evaluateTargetHealth: true,
      },
    });

    new Route53Record(this, "webrtc-dns", {
      zoneId: hostedZone.id,
      name: "webrtc.awestruck.io",
      type: "A",
      alias: {
        name: webrtcNlb.dnsName,
        zoneId: webrtcNlb.zoneId,
        evaluateTargetHealth: true,
      },
    });

    new Route53Record(this, "turn-dns", {
      zoneId: hostedZone.id,
      name: "turn.awestruck.io",
      type: "A",
      alias: {
        name: webrtcNlb.dnsName,
        zoneId: webrtcNlb.zoneId,
        evaluateTargetHealth: true,
      },
    });

    // First, organize all target groups together
    const turnTargetGroup = new LbTargetGroup(this, "awestruck-turn-tg", {
      name: "awestruck-turn-tg",
      port: 3478,
      protocol: "UDP",
      targetType: "ip",
      vpcId: vpc.id,
      healthCheck: {
        enabled: true,
        port: "3479",
        protocol: "HTTP",
        path: "/",
        interval: 10,
        timeout: 5,
        healthyThreshold: 2,
        unhealthyThreshold: 3,
        matcher: "200-299"
      }
    });

    const webrtcTargetGroup = new LbTargetGroup(this, "awestruck-webrtc-tg", {
      name: "awestruck-webrtc-tg",
      port: 8080,
      protocol: "TCP",
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
      }
    });

    const clientTargetGroup = new LbTargetGroup(this, "awestruck-client-tg", {
      name: "awestruck-client-tg",
      port: 5173,
      protocol: "TCP",
      targetType: "ip",
      vpcId: vpc.id,
      healthCheck: {
        enabled: true,
        port: "5173",
        protocol: "HTTP",
        path: "/",
        interval: 5,
        timeout: 2,
        healthyThreshold: 2,
        unhealthyThreshold: 3,
        matcher: "200-299"
      }
    });

    // Then define listeners with correct protocols
    // why we need separate listeners with specific protocols:
    // - udp for turn (no ssl)
    // - tcp for webrtc (no ssl)
    // - tls for client (with ssl)
    const turnListener = new LbListener(this, "turn-udp-listener", {
      loadBalancerArn: webrtcNlb.arn,
      port: 3478,
      protocol: "UDP",
      defaultAction: [{
        type: "forward",
        targetGroupArn: turnTargetGroup.arn
      }]
    });

    const webrtcListener = new LbListener(this, "webrtc-tcp-listener", {
      loadBalancerArn: webrtcNlb.arn,
      port: 8080,
      protocol: "TCP",  // Simple TCP, no SSL
      defaultAction: [{
        type: "forward",
        targetGroupArn: webrtcTargetGroup.arn
      }]
    });

    const clientListener = new LbListener(this, "client-https-listener", {
      loadBalancerArn: webrtcNlb.arn,
      port: 443,
      protocol: "TLS",
      certificateArn: sslCertificateArn,
      defaultAction: [{
        type: "forward",
        targetGroupArn: clientTargetGroup.arn
      }]
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
              { name: "TURN_SERVER_HOST", value: "turn.awestruck.io" },
              { name: "TURN_MIN_PORT", value: TURN_MIN_PORT.toString() },
              { name: "TURN_MAX_PORT", value: TURN_MAX_PORT.toString() },
              { name: "TURN_USERNAME", value: "awestruck_user" },
              { name: "TURN_PASSWORD", value: "verySecurePassword1234567890abcdefghijklmnop" }
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
              { containerPort: 3479, hostPort: 3479, protocol: "tcp" },
              // why we need relay port range mapping:
              // - enables dynamic port allocation for media relay
              // - matches security group configuration
              // - required for webrtc streaming through turn
              ...Array.from({ length: TURN_MAX_PORT - TURN_MIN_PORT + 1 }, (_, i) => ({
                containerPort: TURN_MIN_PORT + i,
                hostPort: TURN_MIN_PORT + i,
                protocol: "udp"
              }))
            ],
            environment: [
              { name: "AWESTRUCK_ENV", value: "production" },
              { name: "HEALTH_PORT", value: "3479" },
              { name: "TURN_REALM", value: "awestruck.io" },
              { name: "PUBLIC_IP", value: turnElasticIp.publicIp },
              { name: "TURN_USERNAME", value: "awestruck_user" },
              { name: "TURN_PASSWORD", value: "verySecurePassword1234567890abcdefghijklmnop" },
              { name: "USERS", value: "awestruck_user=verySecurePassword1234567890abcdefghijklmnop" },
              { name: "TURN_MIN_PORT", value: TURN_MIN_PORT.toString() },
              { name: "TURN_MAX_PORT", value: TURN_MAX_PORT.toString() }
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

    // why we need to expose the elastic ip:
    // - helps with debugging turn connectivity
    // - enables direct ip access if needed
    // - supports manual testing
    new TerraformOutput(this, "turn-elastic-ip", {
      value: turnElasticIp.publicIp,
      description: "Elastic IP address for TURN server",
    });

    // Update turn-service to use simplified load balancer configuration
    new EcsService(this, "turn-service", {
      name: "awestruck-turn-service",
      cluster: ecsCluster.arn,
      taskDefinition: turnTaskDefinition.arn,
      desiredCount: 1,
      launchType: "FARGATE",
      forceNewDeployment: true,
      networkConfiguration: {
        assignPublicIp: true,
        subnets: [subnet.id],
        securityGroups: [securityGroup.id],
      },
      loadBalancer: [{
        targetGroupArn: turnTargetGroup.arn,
        containerName: "turn-server",
        containerPort: 3478
      }],
      dependsOn: [turnListener, turnTargetGroup]
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
            width: 24,
            height: 6,
            properties: {
              query: `SOURCE '${webrtcLogGroup.name}' | 
                fields @timestamp, @message |
                filter @message like /\\[ICE\\]|\\[WebRTC\\]|\\[DTLS\\]/ |
                sort @timestamp desc |
                limit 100`,
              region: awsRegion,
              title: "Connection Event Timeline",
              view: "table"
            },
          },
          {
            type: "log",
            x: 0,
            y: 6,
            width: 12,
            height: 6,
            properties: {
              query: `SOURCE '${webrtcLogGroup.name}' | 
                fields @timestamp, @message |
                filter @message like /Processing candidate/ |
                parse @message /Processing candidate: protocol=(?<protocol>\\S+) address=(?<address>\\S+) port=(?<port>\\d+) priority=(?<priority>\\d+) type=(?<type>\\S+)/ |
                stats count(*) as count by type, protocol |
                sort count desc`,
              region: awsRegion,
              title: "ICE Candidate Distribution",
              view: "table"
            },
          },
          {
            type: "log",
            x: 12,
            y: 6,
            width: 12,
            height: 6,
            properties: {
              query: `SOURCE '${webrtcLogGroup.name}' | 
                fields @timestamp, @message |
                filter @message like /\\[AUDIO\\]|\\[GST\\]|Pipeline|JACK/ |
                sort @timestamp desc |
                limit 100`,
              region: awsRegion,
              title: "Audio Pipeline Events",
              view: "table"
            },
          },
          {
            type: "log",
            x: 0,
            y: 12,
            width: 12,
            height: 6,
            properties: {
              query: `SOURCE '${turnLogGroup.name}' | 
                fields @timestamp, @message |
                filter @message like /Authentication|allocation|ERROR/ |
                sort @timestamp desc |
                limit 100`,
              region: awsRegion,
              title: "TURN Server Events",
              view: "table"
            },
          },
          {
            type: "log",
            x: 12,
            y: 12,
            width: 12,
            height: 6,
            properties: {
              query: `SOURCE '${webrtcLogGroup.name}' | 
                fields @timestamp, @message |
                parse @message /sid_(?<session_id>[^_]+)/ |
                stats count(*) as event_count by session_id |
                sort event_count desc |
                limit 10`,
              region: awsRegion,
              title: "Session Activity",
              view: "table"
            },
          },
          {
            type: "log",
            x: 0,
            y: 18,
            width: 24,
            height: 6,
            properties: {
              query: `SOURCE '${webrtcLogGroup.name}' | 
                fields @timestamp, @message |
                filter @message like /ERROR|WARN|failed|disconnected/ |
                sort @timestamp desc |
                limit 100`,
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

    new EcsService(this, "awestruck-webrtc-service", {
      name: "awestruck-webrtc-service",
      cluster: ecsCluster.arn,
      taskDefinition: webrtcTaskDefinition.arn,
      desiredCount: 1,
      launchType: "FARGATE",
      forceNewDeployment: true,
      networkConfiguration: {
        assignPublicIp: true,
        subnets: [subnet.id],
        securityGroups: [securityGroup.id],
      },
      loadBalancer: [
        {
          targetGroupArn: webrtcTargetGroup.arn,
          containerName: "server-arm64",
          containerPort: 8080
        }
      ],
      dependsOn: [webrtcListener, webrtcTargetGroup]
    });

    // why we need a client log group:
    // - centralizes frontend application logs
    // - enables monitoring of client-side errors
    // - maintains consistent logging across services
    const clientLogGroup = new CloudwatchLogGroup(this, "client-log-group", {
      name: `/ecs/client`,
      retentionInDays: 30,
    });

    // why we need a client task definition:
    // - runs our vite/react frontend in production
    // - configures environment for client container
    // - connects to webrtc service for api calls
    const clientTaskDefinition = new EcsTaskDefinition(
      this,
      "awestruck-client-task-definition",
      {
        family: "client",
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
            name: "client",
            image: `${awsAccountId}.dkr.ecr.${awsRegion}.amazonaws.com/po-studio/awestruck/services/client:latest`,
            portMappings: [
              { containerPort: 5173, hostPort: 5173, protocol: "tcp" }
            ],
            environment: [
              { name: "NODE_ENV", value: "production" },
              // why we need absolute urls:
              // - ensures correct service discovery in production
              // - prevents nginx upstream resolution issues
              // - maintains consistent api endpoints
              { name: "VITE_API_URL", value: "https://webrtc.awestruck.io" },
              { name: "NGINX_API_URL", value: "https://webrtc.awestruck.io" }
            ],
            healthCheck: {
              command: ["CMD-SHELL", "curl -f http://localhost:5173/ || exit 1"],
              interval: 30,
              timeout: 5,
              retries: 3,
              startPeriod: 60
            },
            logConfiguration: {
              logDriver: "awslogs",
              options: {
                "awslogs-group": clientLogGroup.name,
                "awslogs-region": awsRegion,
                "awslogs-stream-prefix": "client",
              },
            }
          }
        ]),
      }
    );

    // why we need a client ecs service:
    // - runs and manages client containers
    // - connects to load balancer
    // - ensures high availability
    new EcsService(this, "awestruck-client-service", {
      name: "awestruck-client-service",
      cluster: ecsCluster.arn,
      taskDefinition: clientTaskDefinition.arn,
      desiredCount: 1,
      launchType: "FARGATE",
      forceNewDeployment: true,
      networkConfiguration: {
        assignPublicIp: true,
        subnets: [subnet.id],
        securityGroups: [securityGroup.id],
      },
      loadBalancer: [{
        targetGroupArn: clientTargetGroup.arn,
        containerName: "client",
        containerPort: 5173
      }],
      dependsOn: [clientListener, clientTargetGroup]
    });
  }
}

const app = new App();
new AwestruckInfrastructure(app, "infra");
app.synth();