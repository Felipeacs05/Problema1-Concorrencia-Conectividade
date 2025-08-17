# Estágio 1: O "Builder" - Usa a imagem completa do Go para compilar nosso código.
# AS builder' dá um nome a este estágio.
FROM golang:1.25-alpine AS builder

# Define o diretório de trabalho dentro do contêiner.
WORKDIR /app

# Copia os arquivos de módulo primeiro. Isso é um truque de cache do Docker.
# Se as dependências não mudarem, o Docker reutiliza esta camada em builds futuros.
COPY go.mod ./
RUN go mod download

# Copia todo o código-fonte do nosso projeto (servidor ou cliente).
COPY ./cliente/ .

# O comando de compilação.
# 'CGO_ENABLED=0' e 'GOOS=linux' garantem que ele crie um executável
# independente para Linux, que é o SO dentro do contêiner.
# '-o /main' define o nome do arquivo de saída como 'main'.
RUN CGO_ENABLED=0 GOOS=linux go build -o /main .

# Estágio 2: O "Final" - Usa uma imagem minúscula, sem nenhuma ferramenta extra.
# A imagem 'alpine' é uma distro Linux muito pequena e segura.
FROM alpine:latest

# Copia APENAS o arquivo executável '/main' que foi compilado no estágio 'builder'.
COPY --from=builder /main /main

# O comando que será executado quando o contêiner iniciar.
CMD ["/main"]