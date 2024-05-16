# Svelte, Node.js, and MySQL

This sample project demonstrates how to deploy a full-stack application using Svelte for the frontend, Node.js for the backend, and MySQL for the database. The project uses Docker to containerize the services, making it easy to run in both development and production environments.

## NOTE

This sample showcases how you could deploy a full-stack application with Defang and Svelte and NodeJS. However, it deploys mysql db as a defang service. Defang [services](https://12factor.net/processes) are ephemeral and should not be used to run stateful workloads in production as they will be reset on every deployment. For production use cases you should use a managed database like RDS, Aiven, or others. In the future, Defang will help you provision and connect to managed databases.

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

For development, we use a local container. This can be seen in the compose.yml file and the server.js file where we create a pool of connections. To run the sample locally after clonging the respository, you can run on docker by doing _docker compose up --build_ or run without using Docker by doing the following:

1. run npm install to install the nodejs dependencies
2. create an .env file on the svelte directory specifying the appropriate environment variables.
3. run npm start

### Editing the database/permissions etc.

If you want to edit the database, such that you can deploy them to production, you should [install the mySQL CLI](https://dev.mysql.com/doc/mysql-shell/8.0/en/mysql-shell-install-linux-quick.html) and mySQL workbench to gain access to a GUI so that you can make your changes to the database. After running defang compose up these changes will be reflected.

## Deploying

1. Open the terminal and type `defang login`
2. Type `defang compose up` in the CLI.
3. Your app will be running within a few minutes.
