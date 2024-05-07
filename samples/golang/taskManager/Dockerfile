# Start from the official Go image
FROM golang:latest
# used since no external modules were used
ENV GO111MODULE=off

# Set the Current Working Directory inside the container
WORKDIR /app 

# Copy the source code from the current directory to the Working Directory inside the container
COPY . .

# Build the Go app
RUN go build -o main .

# Expose port 8080 to the outside world
EXPOSE 8080

# Command to run the executable
CMD ["./main"]