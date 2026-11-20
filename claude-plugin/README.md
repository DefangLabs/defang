# Defang plugin for Claude Code

Deploy a Docker Compose app to your own AWS, Google Cloud or Azure account from a Claude Code session.

## What it adds

- **`/defang:deploy`**: a skill that walks Claude through a deployment with the Defang CLI. It checks the CLI is installed, signs you in, finds or writes the Compose file, picks or creates a stack, sets the config values your app needs, deploys, and checks the result.
- **The Defang MCP server**: tools for deploying, listing services, reading logs and managing config. It starts on its own once the Defang CLI is installed.

You can run `/defang:deploy`, or just ask in your own words, for example "deploy this to our AWS account".

## Install

```
/plugin marketplace add DefangLabs/defang
/plugin install defang@defang-skills
```

## What you need

- A Defang account. The skill runs `defang login`, which opens a sign-in link in your browser.
- Access to the cloud account you want to deploy into (AWS, Google Cloud or Azure), set up the way that cloud's own tools expect.
- The Defang CLI. If it is missing, the skill installs it with `npm install -g defang`, Homebrew or the install script.

## Where things run

Your app is built and deployed inside your own cloud account, and that account is billed for what it runs. The Compose file, and the cloud account and region it targets, are sent to Defang. Your cloud credentials stay on your machine.

## More

- Docs: https://docs.defang.io
- Source and issues: https://github.com/DefangLabs/defang
