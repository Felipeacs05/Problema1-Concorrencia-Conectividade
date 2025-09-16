# ===================== BAREMA ITEM 10: EMULAÇÃO =====================
# Dockerfile para o cliente de teste de estresse

# BAREMA ITEM 10: EMULAÇÃO - Estágio 1: Build da aplicação
FROM golang:1.22-alpine AS builder
WORKDIR /app

# Copia código fonte e compila
COPY . .
RUN go mod tidy
RUN CGO_ENABLED=0 GOOS=linux go build -o /main ./cliente_estresse

# BAREMA ITEM 10: EMULAÇÃO - Estágio 2: Imagem final
FROM alpine:latest

# Copia binário compilado
COPY --from=builder /main /main

# BAREMA ITEM 9: TESTES - Entrypoint para executar testes de estresse
ENTRYPOINT ["/main"]
