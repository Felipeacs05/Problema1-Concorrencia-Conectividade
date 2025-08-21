package main

import (
	"encoding/json" 
	"fmt"
	"net"
	"meujogo/protocolo"
)

func handleConnection(conn net.Conn){
	defer conn.Close()

	fmt.Printf("[SERVIDOR] Nova conexão de %s\n", conn.RemoteAddr().String())

	//Ler a conexão
	decoder := json.NewDecoder(conn)

	for{
		var msg protocolo.Mensagem

		err := decoder.Decode(&msg)
		if err != nil {
			// Se o erro for 'io.EOF', significa que o cliente desconectou de forma limpa.
			fmt.Printf("[SERVIDOR] Conexão com %s fechada.\n", conn.RemoteAddr().String())
			return // Encerra a goroutine.
		}

		fmt.Printf("[SERVIDOR] JSON recebido: %+v\n", msg)

		switch msg.Comando {
		case "LOGIN":
			fmt.Println("[SERVIDOR] Comando de LOGIN recebido")
		case "CRIAR_SALA":
			fmt.Println("[SERVIDOR] Comando de CRIAR_SALA recebido.")
		default:
			fmt.Printf("[SERVIDOR] Comando desconhecido recebido: %s\n", msg.Comando)	
		}

	}
}
func main(){
	fmt.Println("Executando o código do servidor...")

	endereco := ":65432"

	listener, err := net.Listen("tcp", endereco)
	if err != nil {
		fmt.Printf("[SERVIDOR] Erro fatal ao iniciar: %s\n", err)
		return
	}

	defer listener.Close()
	fmt.Printf("[SERVIDOR] Servidor ouvindo na porta: %s\n", endereco)

	for {
		conn, err := listener.Accept()
		if err != nil{
			fmt.Printf("[SERVIDOR] Erro ao aceitar nova conexão: %s\n", err)
			continue
		}

		go handleConnection(conn)
	}
}