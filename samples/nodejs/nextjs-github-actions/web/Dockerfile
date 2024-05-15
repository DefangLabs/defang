# Start from the official Node.js LTS image
FROM node:20-alpine

# Set the working directory
WORKDIR /app

# Copy package.json and package-lock.json
COPY package*.json ./

# Install dependencies
RUN npm install

# Copy the rest of your app's source code
COPY . .

# Build the Next.js app
RUN npm run build

# Expose the port that your app runs on
EXPOSE 3000

# Start the app
CMD ["npm", "start"]