# Sveltekit Music List API

This is a project that demonstrate both client side component rendering and hydration as well as serverside rendering with external API route configuration. Furthermore, there is also a mongodb connection (not hosted on the atlas) to cache the queried results.

## Essential Setup Files

1. A <a href="https://docs.docker.com/develop/develop-images/dockerfile_best-practices/">Dockerfile</a>.
2. A <a href="https://docs.defang.io/docs/concepts/compose">compose file</a> to define and run multi-container Docker applications (this is how Defang identifies services to be deployed).

## Prerequisite

1. Download <a href="https://github.com/defang-io/defang">Defang CLI</a>
2. If you are using <a href="https://docs.defang.io/docs/concepts/defang-byoc">Defang BYOC</a>, make sure you have properly <a href="https://docs.aws.amazon.com/cli/latest/userguide/cli-chap-configure.html">authenticated your AWS account (optional)</a>
   In this sample, there is a environment variable calle MONGODB_URI, please ensure this is set correctly.

## A Step-by-Step Guide

1. Open the terminal and type `defang login`
2. Type `defang compose up` in the CLI
3. Your app should be up and running with Defang in minutes!
