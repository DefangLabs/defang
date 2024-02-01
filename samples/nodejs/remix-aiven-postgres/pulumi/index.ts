import * as defang from "@defang-io/pulumi-defang/lib";
import * as aiven from "@pulumi/aiven";
import * as pulumi from "@pulumi/pulumi";
import { interpolate } from "@pulumi/pulumi";

const config = new pulumi.Config();

const project = new aiven.Project("remix-notes-ui", {
  project: "remix-notes-ui",
  parentId: config.requireSecret("aivenOrganizationId"),
  billingGroup: config.requireSecret("aivenBillingGroupId"),
});

const postgres = new aiven.Pg(
  "remix-notes-pg-service",
  {
    plan: "hobbyist",
    project: project.project,
  },
  { dependsOn: [project] }
);

const database = new aiven.PgDatabase(
  "remix-notes-pg-database",
  {
    databaseName: "remixnotes",
    project: project.project,
    serviceName: postgres.serviceName,
  },
  {
    dependsOn: [postgres],
  }
);

const service = new defang.DefangService(
  "remix-notes-ui",
  {
    build: {
      dockerfile: "./Dockerfile",
      context: "../remix",
    },
    ports: [
      {
        target: 3000,
        mode: "ingress",
        protocol: "http",
      },
    ],
    secrets:[
        {
            source: 'DATABASE_URL',
            value: interpolate`postgresql://${postgres.serviceUsername}:${postgres.servicePassword}@${postgres.serviceHost}:${postgres.servicePort}/${database.databaseName}?schema=public`
        }
    ],
  },
  {
    dependsOn: [database],
  }
);
