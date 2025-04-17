# ![Defang](https://raw.githubusercontent.com/DefangLabs/defang-assets/main/Logos/Element_Wordmark_Slogan/JPG/Dark_Colour_Glow.jpg)

# Defang MCP Server

This folder contains a Model Context Protocol (MCP) server with built-in Defang tools (`deploy`, `destroy`, `login`, `services`) to allow users to manage their services with AI coding agents in a supported IDE.

Below is an installation guide to get started.

## Installation

Connect the MCP server with your IDE by running the following command in your terminal:

```bash
defang mcp setup --client=<your-ide>
```

Replace `<your-ide>` with the name of your preferred IDE. See our list of [Supported IDEs](#supported-ides).

After setup, you can start the MCP server with the command:

```bash
defang mcp serve
```

Once the server is running, you can access the Defang MCP tools directly through the AI agent chat in your IDE.

## Supported IDEs

### Cursor

```bash
defang mcp setup --client=cursor
```

### Windsurf

```bash
defang mcp setup --client=windsurf
```

### VSCode

```bash
defang mcp setup --client=vscode
```

### Claude Desktop

(While this is not an IDE in the traditional sense, it can support MCP servers.)

```bash
defang mcp setup --client=claude
```

## MCP Tools

Below are the tools available in the Defang MCP Server.

### `deploy`

The `deploy` tool scans your project directory for Dockerfiles and `compose.yaml` files, then deploys the detected service(s) using Defang. You can monitor the deployment process in the Defang Portal.

### `destroy`

Given a project name or directory, the `destroy` tool identifies any services deployed with Defang and terminates them. If no services are found, it will display an appropriate message.

### `login`

The `login` tool will open a browser to the Portal to authenticate to Defang. Once you are logged in, a session token is stored, and you will not need to log in again until the session expires or you manually log out.

### `services`

The `services` tool displays the details of all your services that are currently deployed with Defang. It shows the Service Name, Deployment ID, Public URL and Service Status. If there are no services found, it will display an appropriate message.
