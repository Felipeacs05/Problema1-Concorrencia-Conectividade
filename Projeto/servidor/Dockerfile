# ===================== BAREMA ITEM 10: EMULAÇÃO =====================
# Dockerfile para o servidor do jogo de cartas multiplayer
# Otimizado para alta concorrência e performance

# BAREMA ITEM 10: EMULAÇÃO - Estágio 1: Build da aplicação
FROM golang:1.22-alpine AS builder
WORKDIR /app

# Copia arquivos de dependências e baixa módulos
COPY go.mod go.sum* ./
RUN go mod download

# Copia código fonte e compila
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /main ./servidor

# BAREMA ITEM 10: EMULAÇÃO - Estágio 2: Imagem final otimizada
FROM alpine:latest

# BAREMA ITEM 6: LATÊNCIA - Configurações de runtime para alta performance
# Usa todos os CPUs disponíveis
ENV GOMAXPROCS=0
# Configuração agressiva do Garbage Collector
ENV GOGC=50
# Limite de memória para o processo Go
ENV GOMEMLIMIT=6GiB

# Copia binário compilado
COPY --from=builder /main /main

# BAREMA ITEM 2: COMUNICAÇÃO - Expõe porta TCP do servidor
EXPOSE 65432

# BAREMA ITEM 10: EMULAÇÃO - Comando para iniciar o servidor
CMD ["/main"]