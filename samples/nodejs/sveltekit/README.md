# Sveltekit + MongoDB

This is a project that demonstrate both client side component rendering and hydration as well as serverside rendering with external API route configuration. Furthermore, there is also a mongodb connection (not hosted on the atlas) to cache the queried results.

## NOTE

This sample showcases how you could deploy a full-stack application with Defang and Django. However, it deploys postgres as a defang service. Defang [services](https://12factor.net/processes) are ephemeral and should not be used to run stateful workloads in production as they will be reset on every deployment. For production use cases you should use a managed database like RDS, Aiven, or others. In the future, Defang will help you provision and connect to managed databases.

## Essential Setup Files

1. Download <a href="https://github.com/defang-io/defang">Defang CLI</a>
2. (optional) If you are using <a href="https://docs.defang.io/docs/concepts/defang-byoc">Defang BYOC</a>, make sure you have properly <a href="https://docs.aws.amazon.com/cli/latest/userguide/cli-chap-configure.html">authenticated your AWS account</a>.
3. (development)<a href = "https://docs.docker.com/engine/install/">Docker CLI</a>
4. <a href = https://nodejs.org/en/download/package-manager> NodeJS</a>

## Prerequisite

1. Download <a href="https://github.com/defang-io/defang">Defang CLI</a>
2. (optional) If you are using <a href="https://docs.defang.io/docs/concepts/defang-byoc">Defang BYOC</a>, make sure you have properly <a href="https://docs.aws.amazon.com/cli/latest/userguide/cli-chap-configure.html">authenticated your AWS account</a>.
3. (development)<a href = "https://docs.docker.com/engine/install/">Docker CLI</a>
4. <a href = https://nodejs.org/en/download/package-manager> NodeJS</a>

## Development

For development, we use a local container. This can be seen in the compose.yml and /src/routes/api/songs/+server.js file and the server.js file where we create a pool of connections. To run the sample locally after clonging the respository, you can run on docker by doing

1.  docker compose up --build

## A Step-by-Step Guide

1. Open the terminal and type `defang login`
2. Type `defang compose up` in the CLI
3. Your app should be up and running with Defang in minutes!
