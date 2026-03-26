# Multi-stage Dockerfile for ralphglasses
# Build: docker build -t ralphglasses:latest .
# Run:   docker run --rm -it -v $(pwd):/workspace ralphglasses:latest --scan-path /workspace

# Stage 1: Build
FROM golang:1.26-alpine AS builder

RUN apk --no-cache add git

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /ralphglasses .

# Stage 2: Runtime
FROM alpine:3.21

RUN apk --no-cache add ca-certificates git

COPY --from=builder /ralphglasses /usr/local/bin/ralphglasses

ENTRYPOINT ["ralphglasses"]
