package main

import (
	"bufio" // Para o Scanner
	"encoding/json" // Para Marshalling/Unmarshallin
	"fmt"
	"net"
	"meujogo/protocolo"
)

func handleConnection(conn net.Conn){
	defer conn.Close()

	fmt.Printf("[SERVIDOR] Nova conexão de %s\n", conn.RemoteAddr().String())

	//Ler a conexão
	scanner := bufio.NewScanner(conn)

	for scanner.Scan(){
		jsonTexto := scanner.Text()

		var msg protocolo.Mensagem

		err := json.Unmarshal([]byte(jsonTexto), &msg)
		if err != nil {
			fmt.Printf("[SERVIDOR] Erro ao decodificar JSON: %s\n", err)
			continue
		}

		switch msg.Comando {
		case "LOGIN":
			fmt.Println("[SERVIDOR] Comando de LOGIN recebido")
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