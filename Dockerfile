# Start from the official Go image for building
FROM golang:1.23-alpine AS builder

# Set the working directory inside the container
WORKDIR /app

# Copy go.mod and go.sum to cache dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy the source code
COPY . .

WORKDIR /app/cmd

# Build the Go binary statically
RUN CGO_ENABLED=0 GOOS=linux go build -o main .

# Final minimal image
FROM alpine:latest

# Copy the binary from builder
COPY --from=builder /app/cmd/main /main

# Expose the port your app uses (example 8080)
EXPOSE 3030

# Run the binary
CMD ["/main"]
