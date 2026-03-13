# Build stage
FROM golang:1.23 AS builder
WORKDIR /src

# Cache modules
COPY go.mod go.sum ./
RUN go mod download

# Copy source
COPY . .

# Build statically
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -ldflags='-s -w' -o /out/coupon-import ./cmd/api

# Runtime stage
FROM gcr.io/distroless/static:nonroot

# Copy binary and assets
COPY --from=builder /out/coupon-import /coupon-import
COPY --from=builder /src/migrations /migrations
COPY --from=builder /src/openapi.yaml /openapi.yaml
COPY --from=builder /src/docs /docs

ENV PORT=9090
EXPOSE 9090

USER nonroot:nonroot

ENTRYPOINT ["/coupon-import"]
