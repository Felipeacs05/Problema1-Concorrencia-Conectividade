package main

import (
	"encoding/json"
	"fmt"
	"meujogo/protocolo"
	"net"
	"time"
)

func main(){
	fmt.Println("Executando o código do cliente...")

	endereco := "servidor:65432"

	time.Sleep(2*time.Second)

	conn, err := net.Dial("tcp", endereco)
	if err != nil{
		fmt.Printf("[CLIENTE] Não foi possível conectar ao servidor: %s", err)
		return
	}

	defer conn.Close()
	fmt.Printf("[CLIENTE] Conectado ao servidor em %s\n", endereco)

	encoder := json.NewEncoder(conn)

	dados := protocolo.DadosLogin{
		Nome: "Felipe",
		Senha: "Felipe123",
		Id: "1",
	}

	msg := protocolo.Mensagem{
		Comando: "LOGIN",
		Dados: dados,
	}

	err = encoder.Encode(msg)

	fmt.Printf("Cliente enviando mensagem para servidor: %s", encoder)
	if err != nil{
		fmt.Printf("Erro ao enviar mensagem: %s", err)
		return
	}

	fmt.Println("[CLIENTE] Mensagem de LOGIN enviada com sucesso")
}