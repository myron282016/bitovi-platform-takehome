# Stage 1: Build the Go application
FROM golang:1.22-alpine AS builder

WORKDIR /app

COPY go.mod ./
COPY main.go ./

RUN CGO_ENABLED=0 GOOS=linux go build \
    -trimpath \
    -ldflags="-s -w" \
    -o /order-processor .

# Stage 2: Minimal production runtime
FROM gcr.io/distroless/static-debian12:nonroot

WORKDIR /

COPY --from=builder /order-processor /order-processor

USER nonroot:nonroot

EXPOSE 8080

ENTRYPOINT ["/order-processor"]
