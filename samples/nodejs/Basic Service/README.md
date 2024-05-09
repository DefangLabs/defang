# Express Request Inspector
This Node.js application, built with Express.js, is designed to inspect and display detailed information about incoming HTTP requests. It supports all HTTP methods and provides insights into the request path, method, headers, query parameters, and body.

## Essential Setup Files
1. A <a href="https://docs.docker.com/develop/develop-images/dockerfile_best-practices/">Dockerfile</a>.
2. A <a href="https://docs.defang.io/docs/concepts/compose">compose file</a> to define and run multi-container Docker applications (this is how Defang identifies services to be deployed).

## Prerequisite
1. Download <a href="https://github.com/defang-io/defang">Defang CLI</a>
2. If you are using <a href="https://docs.defang.io/docs/concepts/defang-byoc">Defang BYOC</a>, make sure you have properly <a href="https://docs.aws.amazon.com/cli/latest/userguide/cli-chap-configure.html">authenticated your AWS account</a>
Plus, make sure that you have properly set your environment variables like `AWS_PROFILE`, `AWS_REGION`, `AWS_ACCESS_KEY_ID`, and `AWS_SECRET_ACCESS_KEY`.
3. a JSON file named package.json should include relevatn metadata to project such as package dependencies, scripts, project verrsions.

## A Step-by-Step Guide
1. Open the terminal and type `defang login`
2. Type `defang compose up` in the CLI
3. Your app should be up and running with Defang in minutes!
