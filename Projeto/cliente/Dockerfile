# servidor/Dockerfile (CORRIGIDO)
FROM golang:1.25-alpine AS builder
WORKDIR /app

# Copia os arquivos de módulo.
COPY go.mod go.sum* ./
RUN go mod download

# Copia TODO o código-fonte para o contêiner.
COPY . .

# Compila o pacote que está DENTRO da pasta 'servidor'.
# O comando é executado a partir de /app.
RUN CGO_ENABLED=0 GOOS=linux go build -o /main ./cliente

# Estágio 2: Final
FROM alpine:latest
COPY --from=builder /main /main
CMD ["/main"]