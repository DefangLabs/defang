# Go Simple Form Submission App
This is a basic Node.js application using the Express framework to demonstrate handling a form submission. The application serves an HTML form where users can input their first name and then greets them personally upon submission. Note alongside your project, you should also include a package.json file that includes the relevant metadata such as package dependencies, scripts, project verrsions so that the Dockerfile can install necessary dependencies.


## Essential Setup Files
1. A <a href="https://docs.docker.com/develop/develop-images/dockerfile_best-practices/">Dockerfile</a>.
2. A <a href="https://docs.defang.io/docs/concepts/compose">compose file</a> to define and run multi-container Docker applications (this is how Defang identifies services to be deployed).

## Prerequisite
1. Download <a href="https://github.com/DefangLabs/defang">Defang CLI</a>
2. If you are using <a href="https://docs.defang.io/docs/concepts/defang-byoc">Defang BYOC</a>, make sure you have properly <a href="https://docs.aws.amazon.com/cli/latest/userguide/cli-chap-configure.html">authenticated your AWS account (optional)</a>

## A Step-by-Step Guide
1. Open the terminal and type `defang login`
2. Type `defang compose up` in the CLI
3. Your app should be up and running with Defang in minutes!
