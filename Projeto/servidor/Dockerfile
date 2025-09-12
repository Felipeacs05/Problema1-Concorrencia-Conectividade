FROM golang:1.22-alpine AS builder
WORKDIR /app

COPY go.mod go.sum* ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -o /main ./servidor

# Estágio 2: Final
FROM alpine:latest

# Configurações para alta concorrência
ENV GOMAXPROCS=0
ENV GOGC=50
ENV GOMEMLIMIT=6GiB

COPY --from=builder /main /main
EXPOSE 65432
CMD ["/main"]