

# Build stage
FROM golang:1.24-alpine AS builder

WORKDIR /app

# Install git
RUN apk add --no-cache git
RUN apt-get update && apt-get install -y libsqlite3-dev

# Copy go mod and sum files
COPY go.mod ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN CGO_ENABLED=1 GOOS=linux GOARCH=amd64 go build -o dora-osg cmd/server/main.go

# Final stage
FROM alpine:latest

WORKDIR /app

# Install ca-certificates for HTTPS
RUN apk --no-cache add ca-certificates

# Copy the binary from builder
COPY --from=builder /app/dora-osg .
COPY --from=builder /app/config/config.yaml.example ./config/config.yaml

# Create data directory
RUN mkdir -p /data/repos /data/zips

# Expose port
EXPOSE 8866

# Run the application
CMD ["./dora-osg"]
