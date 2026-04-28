# Agent Wrapper Dockerfile
# This is a minimal multi-stage build for production deployment
# The base image is scratch for the smallest possible size

# Build stage
FROM golang:1.23-bookworm AS builder

# Set Go proxy for faster downloads in China
ENV GOPROXY=https://goproxy.cn,direct
ENV GOSUMDB=off
ENV HTTP_PROXY= HTTPS_PROXY= http_proxy= https_proxy= ALL_PROXY= all_proxy= NO_PROXY=

WORKDIR /src

# Download dependencies (cached layer)
COPY go.mod go.sum ./
RUN go mod download

# Copy source and build
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -installsuffix cgo -ldflags="-w -s" -o /app/wrapper ./cmd/wrapper

# Final stage - minimal scratch image
FROM scratch

# Copy CA certificates for HTTPS requests
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

# Copy the binary
COPY --from=builder /app/wrapper /wrapper

# Expose the main port
EXPOSE 8080

# Run the wrapper
ENTRYPOINT ["/wrapper"]
