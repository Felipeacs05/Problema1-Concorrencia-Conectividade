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

	dados := protocolo.DadosCriarSala{
		NomeDaSala: "Primeira sala",
	}

	msgLogin := protocolo.Mensagem{
		Comando: "CRIAR_SALA",
		Dados:   dados,
	}

	fmt.Printf("Cliente enviando mensagem para servidor: '%+v'\n", msgLogin)
	err = encoder.Encode(msgLogin)

	if err != nil {
		fmt.Printf("Erro ao enviar mensagem: %s\n", err)
		return
	}

	fmt.Printf("[CLIENTE] Mensagem: '%s' enviada com sucesso\n", msgLogin)

	//Cliente recebe de servidor
	fmt.Println("[CLIENTE] Esperando resposta do servidor...")
	var resposta protocolo.Mensagem
	err = decoder.Decode(&resposta)
	if err != nil {
		fmt.Printf("[CLIENTE] Erro ao receber resposta do servidor: %s\n", err)
		return
	}


	fmt.Printf("[%s] Resposta recebida do servidor: '%+v\n' ", dados.NomeDaSala, resposta)

	

}
