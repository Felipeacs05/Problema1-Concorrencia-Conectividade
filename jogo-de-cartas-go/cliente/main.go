// cliente/main.go
package main

import (
	"fmt"
	"net"
	"strconv"
	"time"
)

func main() {
    // ADICIONAMOS ESTA LINHA PARA IDENTIFICAÇÃO
    fmt.Println("--- EXECUTANDO O CÓDIGO DO CLIENTE ---")

	endereco := "servidor:65432"

    // Pequena pausa para dar tempo ao servidor de iniciar no Docker
    time.Sleep(2 * time.Second)

	conn, err := net.Dial("tcp", endereco)
	if err != nil {
		fmt.Printf("[CLIENTE] Não foi possível conectar ao servidor: %s\n", err)
		return
	}
	defer conn.Close()
	fmt.Printf("[CLIENTE] Conectado ao servidor em %s\n", endereco)

	tempoInicial := time.Now()
	mensagem := strconv.FormatInt(tempoInicial.UnixNano(), 10)
	
	conn.Write([]byte(mensagem))

	buffer := make([]byte, 1024)
	_, err = conn.Read(buffer)
	if err != nil {
		fmt.Printf("[CLIENTE] Erro ao ler resposta: %s\n", err)
		return
	}

	latencia := time.Since(tempoInicial)

	fmt.Println("[CLIENTE] Eco recebido do servidor.")
	fmt.Println("======================================")
	fmt.Printf("LATÊNCIA (IDA E VOLTA): %s\n", latencia)
	fmt.Println("======================================")
}