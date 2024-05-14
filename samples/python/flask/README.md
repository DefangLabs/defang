# A Simple Flask App

This is a sample of a basic Flask TODO app. The items are stored in memory and are lost when the server is restarted, but it should give you a basic idea of how to get started with Flask on Defang. Note that alognside your .py file, include a requirements.txt so that the Dockerfile can install the necessary packages with pip. 

## Essential Setup Files
1. A <a href="https://docs.docker.com/develop/develop-images/dockerfile_best-practices/">Dockerfile</a>.
2. A <a href="https://docs.defang.io/docs/concepts/compose">compose file</a> to define and run multi-container Docker applications (this is how Defang identifies services to be deployed).
3. A <a href="https://docs.docker.com/build/building/context/#dockerignore-files">.dockerignore</a> to ignore files that are not needed in the Docker image or will be generated during the build process.

## Prerequisite
1. Download <a href="https://github.com/defang-io/defang">Defang CLI</a>
2. If you are using <a href="https://docs.defang.io/docs/concepts/defang-byoc">Defang BYOC</a>, make sure you have properly <a href="https://docs.aws.amazon.com/cli/latest/userguide/cli-chap-configure.html">authenticated your AWS account (optional)</a> 

## A Step-by-Step Guide
1. Open the terminal and type `defang login`
2. Type `defang compose up` in the CLI
3. Your app should be up and running with Defang in minutes!
