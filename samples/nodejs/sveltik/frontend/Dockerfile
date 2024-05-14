# Use a specific version of Node.js as the base image
FROM node:alpine

# Set the working directory inside the container
WORKDIR /app

# Copy package.json and package-lock.json (or yarn.lock) to leverage Docker cache
COPY package*.json ./

# Install dependencies in a separate layer to take advantage of Docker caching
RUN npm install

# Copy the rest of the application source code
COPY . .

# Build the application for production
RUN npm run build

# Expose the port the app runs on
EXPOSE 3000

# Set the command to start the app (adjust the command according to how you start your SvelteKit app)
# If serving with Vite in production, use 'npm run preview', if using an adapter, use 'node build'
CMD ["npm", "run", "preview"]
