# ![Defang](https://raw.githubusercontent.com/DefangLabs/defang-assets/main/Logos/Element_Wordmark_Slogan/JPG/Dark_Colour_Glow.jpg)
# defang-mcp
This repository contains a Model Context Protocol (MCP) server with built-in tools to allow users to deploy with [Defang](https://defang.io/) through a supported IDE. Below are the instructions and installation guide to get started.


## Prerequisites
You need to have Golang installed on your machine.

- Install with brew
    ```sh
    brew install go
    ```

- Install with [Go installtion wizard](https://go.dev/doc/install)<br>

<br>

One of the supported IDEs:
  - Cursor
  - Windsurf
  - VSCode 
  - Claude Desktop (while not an IDE, it supports MCP servers)



## Manual Installation
1. Clone this repo and cd into it
```sh
git clone https://github.com/DefangLabs/defang-mcp.git
cd defang-mcp
```

2. Run `go run main.go` to make the server start without any issues

3. Then set up the config file for your IDE with the MCP client you are using with 
```json
{
  "mcpServers": {
  "defang": {
        "command": "<Your-path>/go",
        "args": ["-C", "<Your-path-to-Repository>/defang-mcp", "run", "main.go"]
      }
  }
}
```

Config file locations:

- [Cursor](https://docs.cursor.com/context/model-context-protocol#configuring-mcp-servers): `~/.cursor/mcp.json`
- [Windsurf](https://docs.windsurf.com/windsurf/mcp#adding-a-new-server): `~/.codeium/windsurf/mcp_config.json`
- [VSCode](https://code.visualstudio.com/docs/copilot/chat/mcp-servers#_add-an-mcp-server): `~/.vscode/mcp.json`
- [Claude Desktop](https://modelcontextprotocol.io/quickstart/user): `~/.claude/mcp_config.json`

## Installation

Run 
```sh
 go install github.com/DefangLabs/defang-mcp@latest
```

### [VSCode and VSCode Insiders](https://code.visualstudio.com/)
```sh
code --add-mcp "{\"name\":\"defang\",\"command\":\"~/go/bin/defang-mcp\",\"args\": [\"serve\"]}"
```

### [Claude Desktop](https://claude.ai/)
```sh
claude mcp add defang -- /Users/defang/go/bin/defang-mcp serve
```

### [Windsurf](https://windsurf.com/editor)
- Refer to [Manual Installation](#manual-installation)

### [Cursor](https://cursor.sh/)
- Refer to [Manual Installation](#manual-installation)



