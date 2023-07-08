// import * as aws from "@pulumi/aws";
import { DefangService } from "@defang-io/pulumi-defang/lib";

const service = new DefangService("nginx", {
  // image: "docker.io/nginx:latest",
  build: {
    context: "../nodejs/Basic Service",
  },
  // platform: "linux",
  ports: [{ target: 3000, protocol: "http", mode: "ingress" }],
  // secrets: [
  //   {
  //     source: "secretx",
  //     value: "secretvalue", optional
  //   }
  // ],
  // environment: {
  //   RDS_HOST: "rds.endpoint",
  // },
  deploy: {
    replicas: 1,
    resources: {
      reservations: {
        cpu: 0.25,
        memory: 400,
        // devices: [{ capabilities: ["gpu"], count: 1 }],
      },
    },
  },
});

export const id = service.id;
export const urn = service.urn;
export const fabricDNS = service.fabricDNS;
export const fqdn = service.fqdn;
export const name = service.name;
export const natIPs = service.natIPs;
export const etag = service.etag;

// const securityGroup = new aws.ec2.SecurityGroup("nginx", {});

// const securityGroupRule = new aws.ec2.SecurityGroupRule("allow-nginx", {
//   type: "ingress",
//   fromPort: 80,
//   toPort: 80,
//   protocol: "tcp",
//   cidrBlocks: service.natIPs,
//   securityGroupId: service.securityGroup.id,
// });
