# Dockerfile for Svelte frontend and Node.js backend

# Stage 1: Build Svelte frontend
FROM node:16-alpine as frontend

WORKDIR /frontend

# Copy frontend source code
COPY frontend/package*.json ./
COPY frontend/ ./

# Install dependencies and build the frontend
RUN npm install --legacy-peer-deps && npm run build

# Stage 2: Build Node.js backend
FROM node:16-alpine as backend

WORKDIR /app

# Copy backend source code
COPY backend/package*.json ./
COPY backend/ ./

# Install dependencies
RUN npm install

# Copy the built Svelte files
COPY --from=frontend /frontend/build ./public

# Copy the backend source code
COPY backend/ .

# Expose the application on port 3001
EXPOSE 3001

# Command to run the backend
CMD ["node", "server.js"]
