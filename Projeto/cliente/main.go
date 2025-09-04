// Projeto/cliente/main.go
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"meujogo/protocolo"
	"net"
	"os"
	"strings"
)

func lerServidor(conn net.Conn) {
	decoder := json.NewDecoder(conn)
	for {
		var msg protocolo.Mensagem
		if err := decoder.Decode(&msg); err != nil {
			fmt.Println("\n[CLIENTE] Conexão com servidor perdida.")
			os.Exit(1)
		}

		switch msg.Comando {
		case "PARTIDA_ENCONTRADA":
			var dados protocolo.DadosPartidaEncontrada
			if err := json.Unmarshal(msg.Dados, &dados); err == nil {
				fmt.Printf("\r[SISTEMA] Sala encontrada! Conectado com: %s.\n", dados.OponenteNome)
				fmt.Println("\n------------COMANDOS---------------")
				fmt.Println("/jogar           - Joga a carta do topo do seu baralho.")
				fmt.Println("/comprar         - Compra um pacote de cartas novas.")
				fmt.Println("/chat <mensagem> - Envia uma mensagem no chat da sala.")
				fmt.Println("-----------------------------------")
				fmt.Print("> ")
			}
		case "ATUALIZACAO_JOGO":
			var dados protocolo.DadosAtualizacaoJogo
			if err := json.Unmarshal(msg.Dados, &dados); err == nil {
				fmt.Println("\n\n--- Status da Rodada ---")
				fmt.Println(dados.MensagemDoTurno)
				if len(dados.UltimaJogada) > 0 {
					fmt.Println("Cartas na mesa:")
					for nome, carta := range dados.UltimaJogada {
						fmt.Printf("  - %s jogou: %s\n", nome, carta.Nome)
					}
				}
				fmt.Println("Placar:")
				for nome, contagem := range dados.ContagemCartas {
					fmt.Printf("  - %s: %d cartas\n", nome, contagem)
				}
				fmt.Println("------------------------")
				fmt.Print("> ")
			}
		case "FIM_DE_JOGO":
			var dados protocolo.DadosFimDeJogo
			if err := json.Unmarshal(msg.Dados, &dados); err == nil {
				fmt.Printf("\r\n--- FIM DE JOGO ---\nO vencedor é %s!\n", dados.VencedorNome)
				fmt.Println("Obrigado por jogar! A conexão será encerrada.")
				conn.Close()
			}
		case "RECEBER_CHAT":
			var dadosChat protocolo.DadosReceberChat
			if err := json.Unmarshal(msg.Dados, &dadosChat); err == nil {
				fmt.Printf("\r[%s]: %s\n> ", dadosChat.NomeJogador, dadosChat.Texto)
			}
		case "PACOTE_COMPRADO":
			var dados protocolo.DadosPacoteComprado
			if err := json.Unmarshal(msg.Dados, &dados); err == nil {
				fmt.Println("\r\n[SISTEMA] Pacote comprado com sucesso!")
				for _, carta := range dados.CartasRecebidas {
					fmt.Printf("  - Você recebeu: %s\n", carta.Nome)
				}
				fmt.Printf("Estoque restante na loja: %d\n", dados.EstoqueRestante)
				fmt.Print("> ")
			}
		case "ERRO":
			var dados protocolo.DadosErro
			if err := json.Unmarshal(msg.Dados, &dados); err == nil {
				fmt.Printf("\r[SISTEMA-ERRO] %s\n> ", dados.Mensagem)
			}
		}
	}
}

func main() {
	fmt.Println("--- Jogo de Cartas Multiplayer ---")
	scanner := bufio.NewScanner(os.Stdin)

	fmt.Print("Digite seu nome de usuário: ")
	scanner.Scan()
	nomeJogador := scanner.Text()

	conn, err := net.Dial("tcp", "servidor:65432")
	if err != nil {
		fmt.Printf("Não foi possível conectar: %s\n", err)
		return
	}
	defer conn.Close()
	fmt.Printf("Conectado como '%s'. Procurando sala...\n", nomeJogador)

	encoder := json.NewEncoder(conn)

	dadosLogin := protocolo.DadosLogin{Nome: nomeJogador}
	jsonDadosLogin, _ := json.Marshal(dadosLogin)
	msgLogin := protocolo.Mensagem{Comando: "LOGIN", Dados: jsonDadosLogin}
	encoder.Encode(msgLogin)

	msgFila := protocolo.Mensagem{Comando: "ENTRAR_NA_FILA", Dados: nil}
	encoder.Encode(msgFila)

	go lerServidor(conn)

	for scanner.Scan() {
		entrada := scanner.Text()
		partes := strings.Fields(entrada)
		if len(partes) == 0 {
			fmt.Print("> ")
			continue
		}

		var msg protocolo.Mensagem
		comando := partes[0]

		if comando == "/jogar" {
			msg = protocolo.Mensagem{Comando: "JOGAR_CARTA", Dados: nil}
		} else if comando == "/comprar" {
			msg = protocolo.Mensagem{Comando: "COMPRAR_PACOTE", Dados: nil}
		} else if comando == "/chat" {
			if len(partes) > 1 {
				textoDoChat := strings.Join(partes[1:], " ")
				dadosChat := protocolo.DadosEnviarChat{Texto: textoDoChat}
				jsonDadosChat, _ := json.Marshal(dadosChat)
				msg = protocolo.Mensagem{Comando: "ENVIAR_CHAT", Dados: jsonDadosChat}
			} else {
				fmt.Println("[SISTEMA] Uso: /chat <sua mensagem>")
				fmt.Print("> ")
				continue
			}
		} else {
			// Tratamento para comandos desconhecidos ou chat direto
			fmt.Printf("\rVocê: %s\n> ", entrada)
			continue
		}
		
		if err := encoder.Encode(msg); err != nil {
			fmt.Println("Erro ao enviar mensagem:", err)
		}
	}
}