# Task Manager Application
## Overview
This sample is is a web-based task manager designed to help users manage their tasks efficiently. It allows users to add, delete, and view tasks in a simple and intuitive interface. This application is ideal for anyone looking to enhance their productivity by keeping track of their daily activities.

## Features
Create Tasks: Users can add new tasks with descriptions.
Delete Tasks: Users can remove tasks when they are completed or no longer needed.
View Tasks: Users can view a list of their current tasks.
## Technology
Backend: The application is built with Go (Golang), utilizing the powerful net/http standard library for handling HTTP requests and responses.
Database: MongoDB is used for storing tasks. It is a NoSQL database that offers high performance, high availability, and easy scalability.
Frontend: Basic HTML and JavaScript are used for the frontend to interact with the backend via API calls.
Environment: Designed to run in containerized environments using Docker, which ensures consistency across different development and production environments.

There is a environment variable named MONGO_URI, in the compose file, be sure to put your mongodb URI, i.e. 
mongodb+srv://<username>:<pwd>@host
# Note:
Take note that this is a simulation, projects with databases should not be ran on Defang as the data may be lost if a container shuts down. 