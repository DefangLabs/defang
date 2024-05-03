/**
 * Represents a sample Pulumi script that creates a DefangService.
 * This script matches the configuration in the compose.yml file.
 */
import { DefangService } from "@defang-io/pulumi-defang/lib";

const service1 = new DefangService("service1", {
  build: {
    context: "../nodejs/Basic Service",
  },
  ports: [{ mode: "ingress", target: 3000  }],
});

export const fqdn = service1.fqdn;
