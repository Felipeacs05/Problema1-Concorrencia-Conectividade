package main

import (
	"encoding/json"
	"fmt"
	"bufio"
	"os"
	"meujogo/protocolo"
	"net"
	"time"
)

func lerServidor(conn net.Conn){
	decoder := json.NewDecoder(conn)

	for {
		var msg protocolo.Mensagem
		if err := decoder.Decode(&msg); err != nil {
			fmt.Println("[CLIENTE] Conexão com servidor perdida.")
			return
		}

		switch msg.Comando {
		case "RECEBER_CHAT":
			if dadosMap, ok := msg.Dados.(map[string]interface{}); ok{
				nome := dadosMap["nomeJogador"].(string)
				texto := dadosMap["texto"].(string)
				fmt.Printf("\n%s: %s\n> ", nome, texto)
			}
		}
	}
}

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

	go lerServidor(conn)

	//Enviar servidor
	encoder := json.NewEncoder(conn)

	//Receber servidor
	scanner := bufio.NewScanner(os.Stdin)

	fmt.Print("> ")

	for scanner.Scan(){
		texto := scanner.Text()

		dados := protocolo.DadosEnviarChat{
			Texto: texto,
		}

		msg := protocolo.Mensagem{
			Comando: "ENVIAR_CHAT",
			Dados: dados,
		}

		if err := encoder.Encode(msg); err != nil{
			fmt.Println("Erro ao enviar mensagem: ", err)
		}

		fmt.Print("> ")

	}

}
