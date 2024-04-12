FROM node:20-alpine

# Set the working directory in the container
WORKDIR /app

# Copy the package.json and package-lock.json (or yarn.lock)
COPY package*.json ./

# Install dependencies
RUN npm ci

# Copy the rest of your application's code
COPY . .

# Build your application
RUN npm run build

# Expose the port your app runs on
EXPOSE 3000

# Command to start your app
CMD npm run start
