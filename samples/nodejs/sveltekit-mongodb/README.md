# Sveltekit + MongoDB

This is a project that demonstrate both client side component rendering and hydration as well as serverside rendering with external API route configuration. Furthermore, there is also a mongodb connection (not hosted on the atlas) to cache the queried results.

## NOTE

This sample showcases how you could deploy a full-stack application with Defang and Sveltekit. However, it deploys mongodb as a defang service. Defang [services](https://12factor.net/processes) are ephemeral and should not be used to run stateful workloads in production as they will be reset on every deployment. For production use cases you should use a managed database like RDS, Aiven, or others. In the future, Defang will help you provision and connect to managed databases.

## Essential Setup Files

1. Download [Defang CLI] (https://github.com/defang-io/defang)
2. (optional) If you are using [Defang BYOC] (https://docs.aws.amazon.com/cli/latest/userguide/cli-chap-configure.html) authenticated your AWS account.
3. (optional for local development) [Docker CLI] (https://docs.docker.com/engine/install/)

## Prerequisite

1. Download [Defang CLI] (https://github.com/defang-io/defang)
2. (optional) If you are using [Defang BYOC](https://docs.defang.io/docs/concepts/defang-byoc) make sure you have properly
3. [Docker CLI] (https://docs.docker.com/engine/install/)

4. [NodeJS] (https://nodejs.org/en/download/package-manager)

## Development

For development, we use a local container. This can be seen in the compose.yml and /src/routes/api/songs/+server.js file and the server.js file where we create a pool of connections. To run the sample locally after clonging the respository, you can run on docker by doing

1.  docker compose up --build

## A Step-by-Step Guide

1. Open the terminal and type `defang login`
2. Type `defang compose up` in the CLI
3. Your app should be up and running with Defang in minutes!
