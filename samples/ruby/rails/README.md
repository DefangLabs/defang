
This template is a member list project developed using Ruby on Rails, offering a starting point to help you quickly build your team management system. We have prepared all the essential files for deployment. By spending less than 10 minutes setting up the environment, as detailed in the prerequisites, and executing the commands in our step-by-step guide, your website will be ready to go live to the world!

## NOTE
1. This sample showcases how you could deploy a full-stack application with Defang in Ruby on Rails. However, it uses a SQLite database, which isn't production-ready and will be reset with every deployment. For production use cases you should use a managed database like RDS, Aiven, or others. If you stick to Rail's default SQLite database, your stored data will be lost on every deployment, and in some other cases. In the future, Defang will help you provision and connect to managed databases.

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
