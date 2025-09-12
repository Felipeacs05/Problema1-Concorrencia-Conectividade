# Estágio 1: Build
FROM golang:1.22-alpine AS builder
WORKDIR /app
COPY . .
RUN go mod tidy
RUN CGO_ENABLED=0 GOOS=linux go build -o /main ./cliente_estresse

# Estágio 2: Final
FROM alpine:latest
COPY --from=builder /main /main
ENTRYPOINT ["/main"]
