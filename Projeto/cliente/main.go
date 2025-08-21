package main

import (
	"encoding/json"
	"fmt"
	"meujogo/protocolo"
	"net"
	"time"
)

func main() {
	fmt.Println("Executando o código do cliente...")

	endereco := "servidor:65432"

	time.Sleep(2 * time.Second)

	conn, err := net.Dial("tcp", endereco)
	if err != nil {
		fmt.Printf("[CLIENTE] Não foi possível conectar ao servidor: %s\n", err)
		return
	}

	defer conn.Close()
	fmt.Printf("[CLIENTE] Conectado ao servidor em %s\n", endereco)

	//Enviar servidor
	encoder := json.NewEncoder(conn)

	//Receber servidor
	decoder := json.NewDecoder(conn)

	dados := protocolo.DadosLogin{
		Nome:  "Felipe",
		Senha: "Felipe123",
		Id:    "1",
	}

	msg := protocolo.Mensagem{
		Comando: "CRIAR_SALA",
		Dados:   dados,
	}

	fmt.Printf("Cliente enviando mensagem para servidor: '%+v'\n", encoder)
	err = encoder.Encode(msg)

	if err != nil {
		fmt.Printf("Erro ao enviar mensagem: %s\n", err)
		return
	}

	fmt.Println("[CLIENTE] Mensagem de LOGIN enviada com sucesso\n")

	var resposta protocolo.Mensagem
	err = decoder.Decode(&resposta)
	if err != nil {
		fmt.Printf("[CLIENTE] Erro ao receber resposta do servidor: %s\n", err)
		return
	}

	fmt.Printf("[%s] Resposta recebida do servidor: '%+v\n' ", dados.Nome, resposta)

}
