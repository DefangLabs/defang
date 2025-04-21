# ![Defang](https://raw.githubusercontent.com/DefangLabs/defang-assets/main/Logos/Element_Wordmark_Slogan/JPG/Dark_Colour_Glow.jpg)

# Defang MCP Server

This directory hosts the Model Context Protocol (MCP) Server, equipped with Defang tools (`deploy`, `services`, `destroy`) to help users manage their services seamlessly using AI coding agents within supported IDEs.

Follow the guide below to get started.

## Installation

Make sure you have the [npm package manager](https://docs.npmjs.com/downloading-and-installing-node-js-and-npm) installed, as `npx` commands are required for setup.

To connect the MCP Server to your IDE, execute the following command in your

```bash
npx -y defang@latest mcp setup --client=<your-ide>
```

Replace `<your-ide>` with the name of your [preferred IDE](#supported-ides).

After setup, you can start the MCP Server with the command:

```bash
npx -y defang@latest mcp serve
```

Once the server is running, you can access the Defang MCP tools directly through the AI agent chat in your IDE.

## Supported IDEs

### Cursor

```bash
npx -y defang@latest mcp setup --client=cursor
```

### Windsurf

```bash
npx -y defang@latest mcp setup --client=windsurf
```

### VS Code / VS Code Insiders

```bash
npx -y defang@latest mcp setup --client=vscode
```

### Claude Desktop

While this is not an IDE in the traditional sense, it can support MCP servers.

```bash
npx -y defang@latest mcp setup --client=claude
```

## MCP Tools

Below are the tools available in the Defang MCP Server.

### `deploy`

The `deploy` tool scans your project directory for Dockerfiles and `compose.yaml` files, then deploys the detected service(s) using Defang. You can monitor the deployment process in the Defang Portal.

### `services`

The `services` tool displays the details of all your services that are currently deployed with Defang. It shows the Service Name, Deployment ID, Public URL and Service Status. If there are no services found, it will display an appropriate message.

### `destroy`

Given a project name or directory, the `destroy` tool identifies any services deployed with Defang and terminates them. If no services are found, it will display an appropriate message.

## Additional Information

For more details about the Defang MCP Server, please see our [official documentation](https://docs.defang.io/docs/concepts/mcp).
