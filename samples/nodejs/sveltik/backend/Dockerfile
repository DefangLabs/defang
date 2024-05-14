# Use the official Node.js 16 as a parent image
FROM node:16

# Set the working directory
WORKDIR /app

# Copy package.json and package-lock.json (or yarn.lock)
COPY package*.json ./

# Install dependencies
RUN npm install

# Copy the rest of the application
COPY . .

# Expose the port the app runs on
EXPOSE 3001

# Define the command to run the app
CMD ["node", "script.js"]
