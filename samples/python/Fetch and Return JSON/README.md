# Fetch and Return JSON

This Flask application fetches average interest rates from the Fiscal Data Treasury API. It provides endpoints to check the status of the API and to retrieve the latest average interest rates. Note that alognside your .py file, include a requirements.txt so that the Dockerfile can install the necessary packages with pip. 

## Features
Endpoint to check API status.
Endpoint to fetch average interest rates from the Fiscal Data Treasury API.

## Essential Setup Files
1. A <a href="https://docs.docker.com/develop/develop-images/dockerfile_best-practices/">Dockerfile</a>.
2. A <a href="https://docs.defang.io/docs/concepts/compose">compose file</a> to define and run multi-container Docker applications (this is how Defang identifies services to be deployed). This is optional

## Prerequisite
1. Download <a href="https://github.com/defang-io/defang">Defang CLI</a>
2. If you are using <a href="https://docs.defang.io/docs/concepts/defang-byoc">Defang BYOC</a>, make sure you have properly <a href="https://docs.aws.amazon.com/cli/latest/userguide/cli-chap-configure.html">authenticated your AWS account (optional)</a>

## A Step-by-Step Guide
1. Open the terminal and type `defang login`
2. Type `defang compose up` in the CLI
3. Your app should be up and running with Defang in minutes!
