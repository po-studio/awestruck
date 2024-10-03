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

class AwestruckInfrastructure extends TerraformStack {
  constructor(scope: Construct, id: string) {
    super(scope, id);

    new AwsProvider(this, "AWS", {
      region: "us-east-1",
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
          toPort: 65535,
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
      name: "awestruck-tg",
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
      name: "awestruck-alb",
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

    const taskDefinition = new EcsTaskDefinition(this, "awestruck-task-definition", {
      family: "go-webrtc-server-arm64",
      cpu: "256",
      memory: "512",
      networkMode: "awsvpc",
      requiresCompatibilities: ["FARGATE"],
      executionRoleArn: ecsTaskExecutionRole.arn,
      containerDefinitions: JSON.stringify([
        {
          name: "go-webrtc-server-arm64",
          image: "${AWS_ACCOUNT_ID}.dkr.ecr.us-east-1.amazonaws.com/po-studio/awestruck:latest",
          portMappings: [
            { containerPort: 8080, hostPort: 8080 },
            ...Array.from({ length: 11 }, (_, i) => ({
              containerPort: 10000 + i,
              hostPort: 10000 + i,
              protocol: "udp"
            }))
          ],
          environment: [
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
              "awslogs-group": `/ecs/${this.node.tryGetContext("taskDefinitionFamily")}`,
              "awslogs-region": "us-east-1",
              "awslogs-stream-prefix": "ecs"
            }
          }
        }
      ])
    });

    new CloudwatchLogGroup(this, "awestruck-log-group", {
      name: `/ecs/${this.node.tryGetContext("taskDefinitionFamily")}`,
      retentionInDays: 30,
    });

    new EcsService(this, "awestruck-service", {
      name: "awestruck-service",
      cluster: ecsCluster.arn,
      taskDefinition: taskDefinition.arn,
      desiredCount: 1,
      launchType: "FARGATE",
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
    });

    new LbListener(this, "awestruck-https-listener", {
      loadBalancerArn: alb.arn,
      port: 443,
      protocol: "HTTPS",
      sslPolicy: "ELBSecurityPolicy-2016-08",
      certificateArn: "arn:aws:acm:us-east-1:${AWS_ACCOUNT_ID}:certificate/3fa50879-056c-46a9-9ad5-74af71d719d7",
      defaultAction: [
        {
          type: "forward",
          targetGroupArn: targetGroup.arn,
        },
      ],
    });
  }
}

const app = new App();
new AwestruckInfrastructure(app, "infra");
app.synth();