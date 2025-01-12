import { App } from "cdktf";
import { Construct } from "constructs";
import { TerraformStack } from "cdktf";
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
import { Lb as AlbLoadBalancer } from "@cdktf/provider-aws/lib/lb";
import { LbTargetGroup } from "@cdktf/provider-aws/lib/lb-target-group";
import { LbListener } from "@cdktf/provider-aws/lib/lb-listener";
import { CloudwatchLogGroup } from "@cdktf/provider-aws/lib/cloudwatch-log-group";
import { SsmParameter } from "@cdktf/provider-aws/lib/ssm-parameter";
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
          // WebRTC media ports
          fromPort: 10000,
          toPort: 65535,
          protocol: "udp",
          cidrBlocks: ["0.0.0.0/0"],
        },
        {
          // STUN server port
          fromPort: 3478,
          toPort: 3478,
          protocol: "udp",
          cidrBlocks: ["0.0.0.0/0"],
        },
        {
          // STUN server port (TCP fallback)
          fromPort: 3478,
          toPort: 3478,
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
        interval: 15,
        timeout: 5,
        matcher: "200-299"
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
              ...Array.from({ length: 101 }, (_, i) => ({
                containerPort: 10000 + i,
                hostPort: 10000 + i,
                protocol: "udp",
              })),
            ],
            environment: [
              { name: "DEPLOYMENT_TIMESTAMP", value: new Date().toISOString() },
              { name: "ENVIRONMENT", value: "production" },
              { name: "JACK_NO_AUDIO_RESERVATION", value: "1" },
              { name: "JACK_OPTIONS", value: "-R -d dummy" },
              { name: "JACK_SAMPLE_RATE", value: "48000" },
              { name: "GST_DEBUG", value: "2" },
              { name: "JACK_BUFFER_SIZE", value: "2048" },
              { name: "JACK_PERIODS", value: "3" },
              { name: "GST_BUFFER_SIZE", value: "4194304" },
              { name: "OPENAI_API_KEY", value: "{{resolve:ssm:/awestruck/openai_api_key:1}}" },
              { name: "AWESTRUCK_API_KEY", value: "{{resolve:ssm:/awestruck/awestruck_api_key:1}}" }
            ],
            linuxParameters: {
              capabilities: {
                add: ["SYS_NICE", "IPC_LOCK"]
              }
            },
            ulimits: [
              { name: "memlock", softLimit: -1, hardLimit: -1 },
              { name: "stack", softLimit: 67108864, hardLimit: 67108864 },
              { name: "rtprio", softLimit: 99, hardLimit: 99 }
            ],
            logConfiguration: {
              logDriver: "awslogs",
              options: {
                "awslogs-group": logGroup.name,
                "awslogs-region": awsRegion,
                "awslogs-stream-prefix": "ecs",
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

    // TODO: rename to webrtc-service
    // we can later have separate services for supercollider, jack, etc.
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
          containerName: "server-arm64",
          containerPort: 8080,
        },
      ],
      dependsOn: [listener],
    });

    // STUN server task definition
    const stunTaskDefinition = new EcsTaskDefinition(
      this,
      "stun-task-definition",
      {
        family: "stun-server-arm64",
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
            name: "stun-server-arm64",
            image: `${awsAccountId}.dkr.ecr.${awsRegion}.amazonaws.com/po-studio/awestruck/services/stun:latest`,
            portMappings: [
              { containerPort: 3478, hostPort: 3478, protocol: "udp" }
            ],
            environment: [
              { name: "STUN_PORT", value: "3478" }
            ],
            logConfiguration: {
              logDriver: "awslogs",
              options: {
                "awslogs-group": logGroup.name,
                "awslogs-region": awsRegion,
                "awslogs-stream-prefix": "stun",
              },
            }
          }
        ]),
      }
    );

    // STUN server service with public IP
    new EcsService(this, "stun-service", {
      name: "stun-service",
      cluster: ecsCluster.arn,
      taskDefinition: stunTaskDefinition.arn,
      desiredCount: 1,
      launchType: "FARGATE",
      forceNewDeployment: true,
      networkConfiguration: {
        assignPublicIp: true,
        subnets: [subnet1.id, subnet2.id],
        securityGroups: [securityGroup.id],
      }
    });

    // Simple DNS record for STUN server using service discovery
    new Route53Record(this, "stun-dns", {
      zoneId: hostedZone.zoneId,
      name: "stun.awestruck.io",
      type: "A",
      ttl: 60,
      records: ["0.0.0.0"], // This will be updated with the actual IP once the service is running
    });

    // Attach ECR read policy to allow pulling images
    new IamRolePolicyAttachment(this, "ecs-ecr-policy", {
      role: ecsTaskExecutionRole.name,
      policyArn: "arn:aws:iam::aws:policy/AmazonEC2ContainerRegistryReadOnly",
    });
  }
}

const app = new App();
new AwestruckInfrastructure(app, "infra");
app.synth();