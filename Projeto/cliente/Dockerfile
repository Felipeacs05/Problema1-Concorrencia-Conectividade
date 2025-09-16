# ===================== BAREMA ITEM 10: EMULAÇÃO =====================
# Dockerfile para o cliente interativo do jogo de cartas

# BAREMA ITEM 10: EMULAÇÃO - Estágio 1: Build da aplicação
FROM golang:1.22-alpine AS builder
WORKDIR /app

# Copia arquivos de dependências e baixa módulos
COPY go.mod go.sum* ./
RUN go mod download

# Copia código fonte e compila
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /main ./cliente

# BAREMA ITEM 10: EMULAÇÃO - Estágio 2: Imagem final
FROM alpine:latest

# Copia binário compilado
COPY --from=builder /main /main

# BAREMA ITEM 10: EMULAÇÃO - Comando para iniciar o cliente
CMD ["/main"]