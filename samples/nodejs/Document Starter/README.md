This template is a simple document starter developed using Nextra. It is ready to be deployed by Defang, and you could make your project available to the world in just 5 minutes!

## Essential Setup Files
1. A <a href="https://docs.docker.com/develop/develop-images/dockerfile_best-practices/">Dockerfile</a> to describe the basic image of your applications.
2. A <a href="https://docs.defang.io/docs/concepts/compose">docker-compose file</a> to define and run multi-container Docker applications.
3. A <a href="https://docs.docker.com/build/building/context/#dockerignore-files">.dockerignore</a> file to comply with the size limit (10MB).

## Prerequisite
1. Download <a href="https://github.com/defang-io/defang">Defang CLI</a>
2. If you are using <a href="https://docs.defang.io/docs/concepts/defang-byoc">Defang BYOC</a>, make sure you have properly <a href="https://docs.aws.amazon.com/cli/latest/userguide/cli-chap-configure.html">authenticated your AWS account</a>
Plus, make sure that you have properly set your environment variables like `AWS_PROFILE`, `AWS_REGION`, `AWS_ACCESS_KEY_ID`, and `AWS_SECRET_ACCESS_KEY`.

## A Step-by-Step Guide
1. Open the terminal and type `defang login`
2. Type `defang compose up` in the CLI
3. Now your application will be launched
