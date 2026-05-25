# IronClaw Go 多阶段构建
FROM golang:1.26-alpine AS go-builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN make build-go

FROM rust:1.92-alpine AS rust-builder
RUN apk add --no-cache musl-dev protoc
WORKDIR /app
COPY wasm-runtime/ ./wasm-runtime/
COPY proto/ ./proto/
RUN make build-rust

FROM alpine:latest
RUN apk add --no-cache ca-certificates
WORKDIR /app
COPY --from=go-builder /app/bin/ironclaw ./
COPY --from=rust-builder /app/bin/wasm-runtime ./
EXPOSE 8080
CMD ["./ironclaw"]
