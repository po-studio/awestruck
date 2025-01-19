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
          // why we need webrtc media ports:
          // - each session needs exactly one port
          // - port range supports up to 100 concurrent sessions
          // - matches webrtc_manager allocation (10000-10010)
          fromPort: 10000,
          toPort: 10010,
          protocol: "udp",
          cidrBlocks: ["0.0.0.0/0"],
        },
        {
          fromPort: 3478,
          toPort: 3478,
          protocol: "udp",
          cidrBlocks: ["0.0.0.0/0"],
        },
        {
          fromPort: 3479,
          toPort: 3479,
          protocol: "tcp",
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

    const taskDefinition = new EcsTaskDefinition(
      this,
      "awestruck-task-definition",
      {
        family: "server-arm64",
        cpu: "1024",
        memory: "2048",
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
              { containerPort: 8080, hostPort: 8080, protocol: "tcp" },
              // why we map all ports:
              // - each session needs one unique port
              // - supports up to 100 concurrent sessions
              // - matches webrtc_manager port range
              ...Array.from({ length: 11 }, (_, i) => ({
                containerPort: 10000 + i,
                hostPort: 10000 + i,
                protocol: "udp",
              })),
            ],
            environment: [
              { name: "DEPLOYMENT_TIMESTAMP", value: new Date().toISOString() },
              { name: "AWESTRUCK_ENV", value: "production" },
              { name: "JACK_NO_AUDIO_RESERVATION", value: "1" },
              { name: "JACK_PORT_MAX", value: "128" },
              { name: "GST_DEBUG", value: "3" },
              { name: "GST_BUFFER_SIZE", value: "8388608" },
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

    // why we need webrtc media ports:
    // - each session needs exactly one port
    // - port range supports multiple concurrent sessions
    // - matches webrtc_manager port allocation
    const webrtcUdpTargetGroups = Array.from({ length: 101 }, (_, i) => {
      const port = 10000 + i;
      return new LbTargetGroup(this, `awestruck-webrtc-udp-tg-${port}`, {
        name: `awestruck-webrtc-tg-${port}`,
        port: port,
        protocol: "UDP",
        targetType: "ip",
        vpcId: vpc.id,
        healthCheck: {
          enabled: true,
          protocol: "TCP",
          port: "8080",
          healthyThreshold: 3,
          unhealthyThreshold: 5,
          interval: 30,
          timeout: 10
        }
      });
    });

    // why we need a listener per port:
    // - each session gets its own dedicated port
    // - enables proper session isolation
    // - supports up to 100 concurrent sessions
    const webrtcUdpListeners = webrtcUdpTargetGroups.map((tg, i) => {
      const port = 10000 + i;
      return new LbListener(this, `webrtc-udp-listener-${port}`, {
        loadBalancerArn: webrtcNlb.arn,
        port: port,
        protocol: "UDP",
        defaultAction: [{
          type: "forward",
          targetGroupArn: tg.arn,
        }],
      });
    });

    // Update awestruck-service to use both load balancers
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
        // HTTP/HTTPS traffic through ALB
        {
          targetGroupArn: targetGroup.arn,
          containerName: "server-arm64",
          containerPort: 8080,
        },
        // WebRTC UDP traffic through NLB
        // why we need all port mappings:
        // - each target group handles a specific port
        // - matches webrtc_manager port allocation
        // - enables proper session routing
        ...webrtcUdpTargetGroups.map((tg, i) => ({
          targetGroupArn: tg.arn,
          containerName: "server-arm64",
          containerPort: 10000 + i,
        }))
      ],
      dependsOn: [listener, ...webrtcUdpListeners],
    });

    // why we need a separate security group for turn:
    // - isolates turn server network access
    // - allows specific ports for stun/turn protocols
    // - enables proper security monitoring
    const turnSecurityGroup = new SecurityGroup(this, "turn-security-group", {
      name: "awestruck-turn-sg",
      description: "Security group for TURN server",
      vpcId: vpc.id,
      ingress: [
        {
          // stun/turn control port
          fromPort: 3478,
          toPort: 3478,
          protocol: "udp",
          cidrBlocks: ["0.0.0.0/0"],
        },
        {
          // health check port
          fromPort: 3479,
          toPort: 3479,
          protocol: "tcp",
          cidrBlocks: ["0.0.0.0/0"],
        },
        {
          // turn relay ports
          fromPort: 49152,
          toPort: 49252,
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

    // why we need a static elastic ip:
    // - ensures turn server can bind to the external ip
    // - prevents "cannot assign requested address" errors
    // - provides stable ip for ice candidates
    const turnElasticIp1 = new Eip(this, "turn-elastic-ip-1", {
      vpc: true,
      tags: {
        Name: "awestruck-turn-eip-1",
      },
    });

    const turnElasticIp2 = new Eip(this, "turn-elastic-ip-2", {
      vpc: true,
      tags: {
        Name: "awestruck-turn-eip-2",
      },
    });

    // why we need a network load balancer for turn:
    // - supports udp protocol required for turn/stun
    // - preserves client ip addresses for nat traversal
    // - provides lower latency than application load balancer
    const turnNlb = new Lb(this, "awestruck-turn-nlb", {
      name: "awestruck-turn-nlb",
      internal: false,
      loadBalancerType: "network",
      enableCrossZoneLoadBalancing: true,
      // why we need subnet mappings with elastic ips:
      // - automatically associates eips with nlb
      // - provides static ips for each availability zone
      // - enables proper nat traversal across azs
      subnetMapping: [
        {
          subnetId: subnet1.id,
          allocationId: turnElasticIp1.allocationId,
        },
        {
          subnetId: subnet2.id,
          allocationId: turnElasticIp2.allocationId,
        },
      ],
    });

    // Output both Elastic IPs for reference
    new TerraformOutput(this, "turn-elastic-ips", {
      value: {
        az1: turnElasticIp1.publicIp,
        az2: turnElasticIp2.publicIp,
      },
      description: "Elastic IPs for TURN server in each AZ",
    });

    // why we use a single target group for turn:
    // - handles both stun and turn traffic on same port
    // - simplifies load balancer configuration
    // - enables health checks for the service
    const turnUdpTargetGroup = new LbTargetGroup(this, "awestruck-turn-udp-tg", {
      name: "awestruck-turn-udp-tg",
      port: 3478,
      protocol: "UDP",
      targetType: "ip",
      vpcId: vpc.id,
      healthCheck: {
        enabled: true,
        protocol: "TCP",
        port: "3479",
        healthyThreshold: 3,
        unhealthyThreshold: 5,
        interval: 30,
        timeout: 10
      }
    });

    // why we need a single udp listener:
    // - handles both stun and turn protocols on standard port 3478
    // - provides nat traversal and relay capabilities
    // - simplifies client configuration
    const turnUdpListener = new LbListener(this, "turn-udp-listener", {
      loadBalancerArn: turnNlb.arn,
      port: 3478,
      protocol: "UDP",
      defaultAction: [{
        type: "forward",
        targetGroupArn: turnUdpTargetGroup.arn,
      }],
    });

    // why we need dns records for the turn server:
    // - enables client discovery of turn/stun services
    // - allows for future ip changes without client updates
    // - supports geographic dns routing if needed
    new Route53Record(this, "turn-dns", {
      zoneId: hostedZone.zoneId,
      name: "turn.awestruck.io",
      type: "A",
      allowOverwrite: true,
      alias: {
        name: turnNlb.dnsName,
        zoneId: turnNlb.zoneId,
        evaluateTargetHealth: true,
      },
    });

    // why we need a turn log group:
    // - centralizes turn server logs
    // - enables log retention policies
    // - supports cloudwatch monitoring
    const turnLogGroup = new CloudwatchLogGroup(this, "turn-log-group", {
      name: `/ecs/turn-server`,
      retentionInDays: 30,
    });

    // why we need a turn task definition:
    // - runs our pion turn implementation
    // - consistent with other services
    // - enables proper monitoring and scaling
    const turnTaskDefinition = new EcsTaskDefinition(
      this,
      "turn-task-definition",
      {
        family: "turn-server",
        cpu: "1024",
        memory: "2048",
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
              ...Array.from({ length: 101 }, (_, i) => ({
                containerPort: 49152 + i,
                hostPort: 49152 + i,
                protocol: "udp"
              })),
            ],
            environment: [
              { name: "DEPLOYMENT_TIMESTAMP", value: new Date().toISOString() },
              { name: "TURN_REALM", value: "awestruck.io" },
              { name: "TURN_PORT", value: "3478" },
              { name: "HEALTH_CHECK_PORT", value: "3479" },
              { name: "MIN_PORT", value: "49152" },
              { name: "MAX_PORT", value: "49252" },
              { name: "AWESTRUCK_ENV", value: "production" },
              { name: "EXTERNAL_IP", value: turnElasticIp1.publicIp }
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

    // why we need a separate target group for relay ports:
    // - handles media relay traffic on ephemeral ports 49152-49252
    // - enables proper health checks
    // - supports multiple simultaneous relay sessions through port range
    const turnRelayTargetGroup = new LbTargetGroup(this, "awestruck-turn-relay-tg", {
      name: "awestruck-turn-relay-tg",
      port: 49152,
      protocol: "UDP",
      targetType: "ip",
      vpcId: vpc.id,
      healthCheck: {
        enabled: true,
        protocol: "TCP",
        port: "3479",
        healthyThreshold: 3,
        unhealthyThreshold: 5,
        interval: 30,
        timeout: 10
      }
    });

    // why we need a single listener with port range for relay:
    // - handles entire relay port range (49152-49252) in one listener
    // - stays within aws limit of 50 listeners per nlb
    // - maintains same functionality as individual port listeners
    const turnRelayListener = new LbListener(this, "turn-relay-listener", {
      loadBalancerArn: turnNlb.arn,
      port: 49152,
      protocol: "UDP",
      defaultAction: [{
        type: "forward",
        targetGroupArn: turnRelayTargetGroup.arn,
      }],
    });

    // why we need a turn service:
    // - runs our pion turn implementation
    // - enables proper monitoring and scaling
    // - handles both control and relay traffic
    new EcsService(this, "turn-service", {
      name: "awestruck-turn-service",
      cluster: ecsCluster.arn,
      taskDefinition: turnTaskDefinition.arn,
      desiredCount: 2,
      launchType: "FARGATE",
      forceNewDeployment: true,
      networkConfiguration: {
        assignPublicIp: true,
        subnets: [subnet1.id, subnet2.id],
        securityGroups: [turnSecurityGroup.id],
      },
      loadBalancer: [
        {
          targetGroupArn: turnUdpTargetGroup.arn,
          containerName: "turn-server",
          containerPort: 3478,
        },
        {
          targetGroupArn: turnRelayTargetGroup.arn,
          containerName: "turn-server",
          containerPort: 49152,
        }
      ],
      dependsOn: [turnUdpListener, turnRelayListener],
    });

    // why we need dns records for each turn instance:
    // - enables direct instance access if needed
    // - supports client-side load balancing
    // - provides fallback options
    ["turn1", "turn2"].forEach((name, index) => {
      new Route53Record(this, `turn-instance-dns-${index + 1}`, {
        zoneId: hostedZone.zoneId,
        name: `${name}.${hostedZone.name}`,
        type: "A",
        alias: {
          name: turnNlb.dnsName,
          zoneId: turnNlb.zoneId,
          evaluateTargetHealth: true,
        },
      });
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