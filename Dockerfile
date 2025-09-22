FROM golang:1.25-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go test -c -o e2e-tests ./tests

FROM cgr.dev/chainguard/static

WORKDIR /

# Copy the test binary
COPY --from=builder /app/e2e-tests /e2e-tests

# Run the compiled test binary
ENTRYPOINT ["/e2e-tests"]
