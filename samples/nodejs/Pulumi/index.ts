import { DefangService } from "@defang-io/pulumi-defang/lib";

const service = new DefangService("nginx", {
  image: "docker.io/nginx:latest",
  // platform: "linux",
  ports: [{ target: 80, protocol: "http", mode: "ingress" }],
  // secrets: [
  //   {
  //     source: "secretx",
  //     value: "secretvalue", optional
  //   }
  // ],
  // forceNewDeployment: true,
  environment: {
    RDS_HOST: "rds.endpoint",
  },
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
export const fqdn = service.fqdn;
export const fabricDNS = service.fabricDNS;
