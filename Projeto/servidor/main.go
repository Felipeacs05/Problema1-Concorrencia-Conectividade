package main

import (
	"bufio" // Para o Scanner
	"encoding/json" // Para Marshalling/Unmarshallin
	"fmt"
	"net"
	"meujogo/protocolo"
)

func handleConnection(){

}
func main(){
	fmt.Println("Executando o código do servidor...")

	endereco := ":65432"

	listener, err := net.Listen("tcp", endereco)
	if err != nil {
		fmt.Println("[SERVIDOR] Erro fatal ao iniciar: %s\n", err)
		return
	}

	defer listener.Close()
	fmt.Printf("SERVIDOR")
}