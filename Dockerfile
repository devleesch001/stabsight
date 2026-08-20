# Build stage
FROM golang:1.26-alpine AS builder

WORKDIR /src

# Cache dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy source tree
COPY . .

# Compile static binary
ARG TARGETOS
ARG TARGETARCH
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-amd64} go build \
    -ldflags="-s -w -X github.com/devleesch001/stabsight/cmd.version=docker -X github.com/devleesch001/stabsight/cmd.commit=container" \
    -o /out/stabsight ./cmd

# Runtime stage
FROM gcr.io/distroless/static-debian12:nonroot

WORKDIR /app

COPY --from=builder /out/stabsight /app/stabsight
COPY config.example.yaml /etc/stabsight/config.example.yaml

EXPOSE 9090

USER nonroot:nonroot

ENTRYPOINT ["/app/stabsight"]
CMD ["run", "--config", "/etc/stabsight/config.yaml"]
