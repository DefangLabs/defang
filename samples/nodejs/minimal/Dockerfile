# Use an official Node runtime based on Alpine as a parent image
FROM node:18-alpine

# Set the working directory to /app
WORKDIR /app

# Copy the current directory contents into the container at /app
COPY . .

# Run the app when the container launches
ENTRYPOINT [ "node", "main.js" ]
