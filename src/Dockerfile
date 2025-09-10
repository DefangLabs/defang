# Build stage
ARG GO_VERSION=1.24
FROM --platform=${BUILDPLATFORM} golang:${GO_VERSION} AS builder

# These two are automatically set by docker buildx
ARG TARGETARCH
ARG TARGETOS

# Set the working directory
WORKDIR /app

COPY --link go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . /app/

# Build the application
ARG BUILD=./cmd/cli
ARG VERSION
ARG OUTPUT=defang
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -trimpath -buildvcs=false -ldflags="-w -s -X \"main.version=${VERSION}\"" -o "${OUTPUT}" "${BUILD}"

# Final stage
FROM alpine:latest

# Install runtime dependencies
RUN apk add --no-cache ca-certificates tzdata

# Create a non-root user to run the application
RUN adduser -D -h /home/defang defang

# Set working directory
WORKDIR /app

# Copy the binary from the builder stage
COPY --from=builder /app/defang /usr/local/bin/

# Set the user to run the application
USER defang

# For the authentication server
EXPOSE 47071

ENTRYPOINT ["defang"]

CMD ["--help"]


