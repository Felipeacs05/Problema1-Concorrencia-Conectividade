// servidor/main.go
package main

import (
	"fmt"
	"net"
)

// Esta função gerencia a comunicação com um único cliente.
func handleConnection(conn net.Conn) {
	defer conn.Close()
	fmt.Printf("[SERVIDOR] Nova conexão de %s\n", conn.RemoteAddr().String())

	buffer := make([]byte, 1024)

	for {
		n, err := conn.Read(buffer)
		if err != nil {
			fmt.Printf("[SERVIDOR] Conexão com %s perdida: %s\n", conn.RemoteAddr().String(), err)
			return
		}
		fmt.Printf("[SERVIDOR] Recebido de %s: %s\n", conn.RemoteAddr().String(), string(buffer[:n]))
		conn.Write(buffer[:n])
	}
}

func main() {
    // ADICIONAMOS ESTA LINHA PARA IDENTIFICAÇÃO
    fmt.Println("--- EXECUTANDO O CÓDIGO DO SERVIDOR ---")

	endereco := ":65432"
	listener, err := net.Listen("tcp", endereco)
	if err != nil {
		fmt.Printf("[SERVIDOR] Erro fatal ao iniciar: %s\n", err)
		return
	}
	defer listener.Close()
	fmt.Printf("[SERVIDOR] Servidor ouvindo na porta %s\n", endereco)

	for {
		conn, err := listener.Accept()
		if err != nil {
			fmt.Printf("[SERVIDOR] Erro ao aceitar conexão: %s\n", err)
			continue
		}
		// Inicia uma goroutine para cada cliente
		go handleConnection(conn)
	}
}