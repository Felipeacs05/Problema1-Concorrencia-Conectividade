# ===================== BAREMA ITEM 10: EMULAÇÃO =====================
# Dockerfile para o teste de estresse simples

FROM golang:1.21-alpine

WORKDIR /app

# Copia arquivo de teste
COPY estresse.go .

# BAREMA ITEM 10: EMULAÇÃO - Inicializa módulo e compila
RUN go mod init teste-estresse && \
    go build -o estresse estresse.go

# BAREMA ITEM 9: TESTES - Comando para executar teste de estresse
CMD ["./estresse"]
