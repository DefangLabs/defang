# Use an official Go runtime as a parent image
FROM golang:1.20-alpine as builder

# Set the working directory in the builder container
WORKDIR /src

# Copy go.mod and go.sum files to the workspace
COPY go.mod go.sum ./

# Download all dependencies.
RUN go mod download

# Copy the source from the current directory to the working Directory in the builder container
COPY . .

# Build the Go app
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o main .

# Start a new stage from scratch
FROM alpine:latest

RUN apk --no-cache add ca-certificates

WORKDIR /root/

# Copy the binary from builder
COPY --from=builder /src/main .

# Expose port 8080 to the world outside this container
EXPOSE 8080

# Run the binary
ENTRYPOINT ["./main"]
