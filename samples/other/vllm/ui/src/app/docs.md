
### What is Defang?

Defang is a radically simpler way for developers to build, deploy their apps to the cloud. Defang enables you to easily author cloud application in any language, build and deploy to the cloud with a single command, and iterate quickly.

- The [Defang CLI](./getting-started/installing.md) includes an AI-driven assistant that translates natural language prompts to an outline for your project that you can then refine.
- Defang can automatically build and deploy your project with a single command.
    - If you’re new to Defang, you can try deploying to the [Defang Playground](./concepts/defang-playground.md), a hosted environment to learn to use Defang with non-production workloads.
    - Once you’re ready, you can [deploy](./concepts/deployments.md) it to your own cloud account - we call this [Defang BYOC](./concepts/defang-byoc.md). Defang takes care of all the heavy lifting such as configuring networking, security, [observability](./concepts/observability.md) and all the other details that usually slow down the average cloud developer.
- You can also use Defang to easily [publish updates](./concepts/deployments.md#deploying-updates) to your deployed application with zero downtime.

### Features

Defang provides a streamlined experience to develop, deploy, observe, and update your cloud applications. Defang includes the following features:

- Support for [various types of applications](./use-cases/use-cases.md): Web services and APIs, mobile app backends, ML services, hosting LLMs, etc.
- Support for your programming [language of choice](./samples.md): Node.js, Python, Golang, or anything else you can package in a Dockerfile.
- Built-in [AI assistant](./concepts/ai.md) to go from natural language prompt to an outline project
- Automated [Dockerfile builds](./concepts/deployments.md)
- Support for [pre-built Docker containers](./tutorials/deploy-container-using-the-cli.mdx), from public or private image registries
- Ability to express your project configuration using a [Docker Compose YAML](./concepts/compose.md) file
- Ability to manage encrypted [secrets](./concepts/secrets.md) and [configuration](./concepts/configuration.md)
- Pre-configured environments with built-in [security](./concepts/security.md), [networking](./concepts/networking.md), and [observability](./concepts/observability.md)
- [One-command deployments](./getting-started/installing.md)
- Support for [GPUs](./concepts/resources.md)
- Support for Infra-as-Code via the [Defang Pulumi provider](./concepts/pulumi.md)



# Getting Started


### Install the CLI

First, you'll need to install the Defang CLI. The CLI is the primary way to interact with Defang. It allows you to create, deploy, and manage your services. You can find the [different installation methods here](./installing.md).

### Authenticate with Defang

To do pretty much anything with Defang, you'll need to authenticate with the platform. You can do this by running the following command:

```bash
defang login
```

:::info
To learn more about how authentication works in defang, check out the [authenticating page](./authenticating.md).
:::

### Build and Deploy Services

Defang supports various ways of creating and deploying services to the cloud. The following tutorials dive into each one in more detail:

1. [Create an outline using AI](../tutorials/generate-new-code-using-ai.mdx)
2. [Build and deploy your code](../tutorials/deploy-code-compose.mdx)
3. [Deploy existing containers](../tutorials/deploy-container-using-the-cli.mdx)
4. [Deploy using Pulumi](../tutorials/deploy-using-pulumi.mdx)


### Monitor Services

By default, all the output (stdout and stderr) from your app is logged. You can view these logs in real-time. You can view logs for all your services, one service, or even one specific deployment of a service.

- From the CLI:

    ```tsx
    defang tail --name service1
    ```

- From the Defang Portal:

    [https://portal.defang.dev/](https://portal.defang.dev/)


:::info
* To learn more about observability in Defang, check out the [observability page](../concepts/observability.md).
* Note that the Defang Portal only displays services deployed to Defang Playground.
:::


### Update Services

To update your app (for example, updating the base image of your container, or making changes to your code) you can run the `defang compose up` command and it will build and deploy a new version with zero downtime. Your current version of the service will keep running and handling traffic while the new version is being built and deployed. Only after the new version passes the health checks and accepts traffic will the older version be stopped.

:::info
If you are using [compose files](../concepts/compose.md) to define your services, you can add/remove services, make changes to code, etc. When you run `defang compose up`, the update will be diffed against the current state and any necessary changes will be applied to make the current state match the desired state.
:::




# Installing

Defang doesn't require installing anything in your cloud, but you will need to install the [open source](https://github.com/defang-io/defang) Defang command line interface (CLI) to interact with your Defang resources and account.

We offer a few different ways to install the Defang CLI. You can use Homebrew, a bash script, or download the binary directly.

## Using Homebrew

You can easily install the Defang CLI using [Homebrew](https://brew.sh/). Just run the following command in your terminal:

```bash
brew install defang-io/defang/defang
```

## Using a Bash Script

You can install the Defang CLI using a bash script. Just run the following command in your terminal:

```bash
. <(curl -Ls https://s.defang.io/install)
```

The script will try to download the appropriate binary for your operating system and architecture, add it to `~/.local/bin`, and add `~/.local/bin` to your `PATH` if it's not already there, with your permission. If you do not provide permission it will print an appropriate instruction for you to follow to add it manually. You can also customize the installation directory by setting the `INSTALL_DIR` environment variable before running the script.

## Direct Download

You can find the latest version of the Defang CLI on the [releases page](https://github.com/defang-io/defang/releases). Just download the appropriate binary for your operating system and architecture, and put it somewhere in your `PATH`.


# FAQ

### Which cloud/region is the app being deployed to?

- In the [Defang Playground](./concepts/defang-playground.md) the app is deployed to AWS `us-west-2`. In the [Defang BYOC](./concepts/defang-byoc.md) model, the region is determined by your [Defang BYOC Provider](/docs/category/providers) settings.

### Can I bring my own AWS or other cloud account?

- Yes! Please check out the [Defang BYOC](./concepts/defang-byoc.md) documentation for more information.

### On AWS, can I deploy to services such as EC2, EKS, or Lambda?

- The current release includes support for containers only, deployed to ECS. We are still exploring how to support additional execution models such as VMs and functions-as-a-service. However, using our Pulumi provider, it is possible to combine Defang services with other native AWS resources.

### Can I access AWS storage services such as S3 or database services such as RDS? How?

- Yes, you can access whatever other resources exist in the cloud account you are using as a [Defang BYOC](./concepts/defang-byoc.md) Provider.

### Do you plan to support other clouds?

- While we currently support AWS as a [Defang BYOC](./concepts/defang-byoc.md) Provider, we plan to support other clouds in future releases, such as [Azure](./providers/azure.md) and [GCP](./providers/gcp.md).

### Can I run production apps with Defang?

- The [Defang Playground](./concepts/defang-playground.md) is meant for testing and trial purposes only. Deployment of productions apps with [Defang BYOC](./concepts/defang-byoc.md) is not yet supported and disallowed by the [Terms of Service](https://defang.io/terms-service.html). If you are interested in running production apps, please [contact us](https://defang.io/#Contact-us).

### I'm having trouble running the binary on my Mac. What should I do?

- MacOS users will need to allow the binary to run due to security settings:
    1. Attempt to run the binary. You'll see a security prompt preventing you from running it.
    2. Go to System Preferences > Privacy & Security > Security.
    3. In the 'Allow applications downloaded from:' section, you should see a message about Defang being blocked. Click 'Open Anyway'.
    4. Alternatively, select the option "App Store and identified developers" to allow all applications from the App Store and identified developers to run.

## Warnings

### "The folder is not empty. Files may be overwritten."
- This message is displayed when you run `defang generate` and the target folder is not empty. If you proceed, Defang will overwrite any existing files with the same name. If you want to keep the existing files, you should move them to a different folder before running `defang generate` or pick a different target folder.

### "environment variable not found"
- This message is displayed when you run `defang compose up` and the Compose file references an environment variable that is not set. If you proceed, the environment variable will be empty in the container. If you want to set the environment variable, you should set it in the environment where you run `defang compose up`.

### "Unsupported platform"
- This message is displayed when you run `defang compose up` and the Compose file references a platform that is not supported by Defang. Defang Beta only supports Linux operating systems.

### "not logged in"
- This message is displayed when you run `defang compose config` but you are not logged in. The displayed configuration will be incomplete. If you want to see the complete configuration, you should log in first using `defang login`.

### "No port mode was specified; assuming 'host'"
- This message is displayed when you run `defang compose up` and the Compose file declares a `port` that does not specify a port `mode`. By default, Defang will keep the port private. If you want to expose the port to the public internet, you should specify the `mode` as `ingress`:
```
services:
  service1:
    ports:
      - target: 80
        mode: ingress
```

### "Published ports are not supported in ingress mode; assuming 'host'"
- This message is displayed when you run `defang compose up` and the Compose file declares a `port` with `mode` set to `ingress` and `published` set to a port number. Defang does not support published ports in ingress mode. If you want to expose the port to the public internet, you should specify the `mode` as `ingress` and remove the `published` setting.

### "TCP ingress is not supported; assuming HTTP"
- This message is displayed when you run `defang compose up` and the Compose file declares a `port` with `mode` set to `ingress` and `protocol` set to `tcp`. Defang does not support arbitrary TCP ingress and will assume the port is used for HTTP traffic. To silence the warning, remove the `protocol` setting.

### "unsupported compose directive"
- This message is displayed when you run `defang compose up` and the Compose file declares a directive that is not supported by Defang. The deployment will continue, but the unsupported directive will be ignored, which may cause unexpected behavior.

### "no reservations specified; using limits as reservations"
- This message is displayed when you run `defang compose up` and the Compose file declares a `resource` with `limits` but no `reservations`. Defang will use the `limits` as `reservations` to ensure the container has enough resources. Specify `reservations` if you want to silence the warning or reserve a different amount of resources:
```
services:
  service1:
    deploy:
      resources:
        reservations:
          cpus: 0.5
          memory: 512MB
```

### "ingress port without healthcheck defaults to GET / HTTP/1.1"
- This message is displayed when you run `defang compose up` and the Compose file declares an `ingress` with a `port` but no `healthcheck`. Defang will assume the default healthcheck of `GET / HTTP/1.1` to ensure the port is healthy. Specify a `healthcheck` if you want to silence the warning or use a different healthcheck:
```
services:
  service1:
    deploy:
      healthcheck:
        test: ["CMD", "curl", "-f", "http://localhost:80/health"]
```

### "missing memory reservation; specify deploy.resources.reservations.memory to avoid out-of-memory errors"
- This message is displayed when you run `defang compose up` and the Compose file doesn't specify a `memory` reservation. If available, Defang will use the `memory` limit as the `memory` reservation. Specify a `memory` reservation if you want to silence the warning or reserve a different amount of memory:
```
services:
  service1:
    deploy:
      resources:
        reservations:
          memory: 512MB
```

