# Go MongoDB Atlas sample
## Overview
This sample is is a web-based task manager designed to help users manage their tasks efficiently. It allows users to add, delete, and view tasks in a simple and intuitive interface. This application is ideal for anyone looking to enhance their productivity by keeping track of their daily activities. There is a go.mod file that includes dependencies for the Dockerfile to install

## Features
Create Tasks: Users can add new tasks with descriptions.
Delete Tasks: Users can remove tasks when they are completed or no longer needed.
View Tasks: Users can view a list of their current tasks.
## Technology
Backend: The application is built with Go (Golang), utilizing the powerful net/http standard library for handling HTTP requests and responses.
Database: MongoDB is used for storing tasks. It is a NoSQL database that offers high performance, high availability, and easy scalability.
Frontend: Basic HTML and JavaScript are used for the frontend to interact with the backend via API calls.
Environment: Designed to run in containerized environments using Docker, which ensures consistency across different development and production environments.

## Prerequisite
1. Download <a href="https://github.com/defang-io/defang">Defang CLI</a>
2. If you are using <a href="https://docs.defang.io/docs/concepts/defang-byoc">Defang BYOC</a>, make sure you have properly <a href="https://docs.aws.amazon.com/cli/latest/userguide/cli-chap-configure.html">authenticated your AWS account</a>
Plus, make sure that you have properly set all environment variables up
3. There is a environment variable named MONGO_URI for the MONGODB connection string, in the compose file, be sure to put your mongodb URI, i.e. 
mongodb+srv://<username>:<pwd>@host

## A Step-by-Step Guide
1. Open the terminal and type `defang login`
2. Type `defang compose up` in the CLI
3. Your app should be up and running with Defang in minutes!
