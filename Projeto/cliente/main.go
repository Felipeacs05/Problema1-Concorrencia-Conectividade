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
			var dadosChat protocolo.DadosReceberChat

			if err := json.Unmarshal(msg.Dados, &dadosChat); err != nil{
				fmt.Println("[CLIENTE] Erro ao ler dados do chat: ", err)
				continue
			}
			
			fmt.Printf("\r%s: %s\n> ", dadosChat.NomeJogador, dadosChat.Texto)
		case "SALA_CRIADA_SUCESSO":
			var dadosSala protocolo.DadosCriarSala
			if err := json.Unmarshal(msg.Dados, &dadosSala); err == nil {
				fmt.Println("[CLIENTE] Erro ao ler dados do chat: ", err)
				return
			}

			fmt.Println("Parabéns! Acaba de criar uma sala")
		}
	}
}

func main() {
	fmt.Println("--- Jogo de Cartas Multiplayer ---")

	scanner := bufio.NewScanner(os.Stdin)

	fmt.Print("Digite seu nome de usuário: ")

	scanner.Scan()

	nomeJogador := scanner.Text()

	//Conexão Com servidor ----
	endereco := "servidor:65432"

	time.Sleep(2 * time.Second)

	conn, err := net.Dial("tcp", endereco)
	if err != nil {
		fmt.Printf("[CLIENTE] Não foi possível conectar ao servidor: %s\n", err)
		return
	}

	defer conn.Close()
	fmt.Printf("[CLIENTE] Conectado como %s ao servidor em %s\n", nomeJogador, endereco)

	//-------------------------
	
	//Enviar login
	encoder := json.NewEncoder(conn)

	cliente := protocolo.DadosLogin{
		Nome: nomeJogador,
	}

	jsonCliente, err := json.Marshal(cliente)
	if err != nil{
		fmt.Printf("Erro ao empacotar dados de login: %s\n", err)
		return
	}

	msgLogin := protocolo.Mensagem{
		Comando: "LOGIN",
		Dados: jsonCliente,
	}

	if err := encoder.Encode(msgLogin); err != nil{
		fmt.Printf("Erro ao enviar mesangem de Login: %s", err)
	}


	go lerServidor(conn)

	fmt.Print("> ")

	//Iniciar loop do chat
	for scanner.Scan(){
		texto := scanner.Text()

		dados := protocolo.DadosEnviarChat{
			Texto: texto,
		}

		jsonDados, err := json.Marshal(dados)
		if err != nil{
			fmt.Printf("[SERVIDOR] Erro ao empacotar dados para enviar ao servidor: %s\n", err)
		}

		msg := protocolo.Mensagem{
			Comando: "ENVIAR_CHAT",
			Dados: jsonDados,
		}

		if err = encoder.Encode(msg); err != nil{
			fmt.Printf("Erro ao enviar mensagem: %s", err)
		}

		fmt.Print("> ")

	}

}
