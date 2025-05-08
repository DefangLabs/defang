# Build stage
FROM golang:alpine AS builder

# Install required build dependencies
RUN apk add --no-cache git make zip curl bash

# Set the working directory
WORKDIR /app

# Copy source code
COPY . /app/

# Build the application
RUN go build -o defang -ldflags "-X main.version=container-build" ./cmd/cli

# Final stage
FROM alpine:latest

# Install runtime dependencies
RUN apk add --no-cache ca-certificates tzdata

# Set working directory
WORKDIR /app

# Copy the binary from the builder stage
COPY --from=builder /app/defang /usr/local/bin/

EXPOSE 32768

ENTRYPOINT ["defang"]

CMD ["--help"]


