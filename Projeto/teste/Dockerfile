FROM golang:1.21-alpine

WORKDIR /app
COPY estresse.go .

RUN go mod init teste-estresse && \
    go build -o estresse estresse.go

CMD ["./estresse"]
