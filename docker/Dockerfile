# IronClaw Go Orchestrator 多阶段构建
FROM golang:1.24-alpine AS go-builder
ENV GOTOOLCHAIN=auto
RUN apk add --no-cache git make protoc ca-certificates
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN make build-go

FROM alpine:latest
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app
COPY --from=go-builder /app/bin/ironclaw ./
EXPOSE 8080
CMD ["./ironclaw"]
