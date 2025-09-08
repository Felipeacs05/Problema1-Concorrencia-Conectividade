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

var meuNome string
var meuInventario []protocolo.Carta

func printAjuda() {
	fmt.Println("\n------------ COMANDOS ---------------")
	fmt.Println("/buy_pack       - compra 10 cartas (apenas fora da partida)")
	fmt.Println("/jogar <nome da carta> - no lobby: inicia; em jogo: joga carta específica")
	fmt.Println("qualquer coisa escrita fora desses comandos será representado como um chat")
	fmt.Println("-------------------------------------")
	fmt.Print("> ")
}

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
				fmt.Printf("\r[SISTEMA] Sala encontrada! Oponente: %s.\n", dados.OponenteNome)
				printAjuda()
			}

		case "ATUALIZACAO_JOGO":
			var dados protocolo.DadosAtualizacaoJogo
			if err := json.Unmarshal(msg.Dados, &dados); err == nil {
				fmt.Printf("\n--- Status da Rodadfda %d ---\n", dados.NumeroRodada)
				fmt.Println(dados.MensagemDoTurno)
				if len(dados.UltimaJogada) > 0 {
					fmt.Println("Cartas na mesa:")
					for nome, carta := range dados.UltimaJogada {
						fmt.Printf("  - %s jogou: %s %s (Poder: %d)\n", nome, carta.Nome, carta.Naipe, carta.Valor)
					}
				}
				if dados.VencedorJogada != "" {
					if dados.VencedorJogada == "EMPATE" {
						fmt.Println("Vencedor da jogada: EMPATE")
					} else {
						fmt.Printf("Vencedor da jogada: %s\n", dados.VencedorJogada)
					}
				}
				if dados.VencedorRodada != "" {
					if dados.VencedorRodada == "EMPATE" {
						fmt.Printf("Vencedor da rodada %d: EMPATE\n", dados.NumeroRodada)
					} else {
						fmt.Printf("Vencedor da rodada %d: %s\n", dados.NumeroRodada, dados.VencedorRodada)
					}
				}
				fmt.Println("Cartas no inventário:")
				for nome, contagem := range dados.ContagemCartas {
					fmt.Printf("  - %s: %d cartas\n", nome, contagem)
				}
				// Mostra inventário do jogador atual
				if len(meuInventario) > 0 {
					fmt.Println("\nSuas cartas:")
					for _, carta := range meuInventario {
						fmt.Printf("  - %s %s (ID: %s, Poder: %d, Raridade: %s)\n",
							carta.Nome, carta.Naipe, carta.ID, carta.Valor, carta.Raridade)
					}
				}
				fmt.Println("------------------------")
				fmt.Print("> ")
			}

		case "FIM_DE_JOGO":
			var dados protocolo.DadosFimDeJogo
			if err := json.Unmarshal(msg.Dados, &dados); err == nil {
				if dados.VencedorNome == "EMPATE" {
					fmt.Printf("\n=== FIM DE JOGO — EMPATE ===\n")
				} else {
					fmt.Printf("\n=== FIM DE JOGO — VENCEDOR DA PARTIDA: %s ===\n", dados.VencedorNome)
				}
				// volta ao lobby: pode comprar pacotes e reiniciar
				printAjuda()
			}

		case "PACOTE_RESULTADO":
			var r protocolo.ComprarPacoteResp
			if err := json.Unmarshal(msg.Dados, &r); err == nil {
				// Atualiza inventário
				meuInventario = append(meuInventario, r.Cartas...)

				n := make([]string, 0, len(r.Cartas))
				for _, c := range r.Cartas {
					n = append(n, fmt.Sprintf("%s(poder = %d)", c.Nome, c.Valor))
				}
				fmt.Printf("\n[Pacote] Você recebeu: %s | estoque=%d\n", strings.Join(n, ", "), r.EstoqueRestante)
				fmt.Print("> ")
			}

		case "RECEBER_CHAT":
			var dadosChat protocolo.DadosReceberChat
			if err := json.Unmarshal(msg.Dados, &dadosChat); err == nil {
				prefix := dadosChat.NomeJogador
				if dadosChat.NomeJogador == meuNome {
					prefix = "[VOCÊ]"
				}
				fmt.Printf("\r%s: %s\n> ", prefix, dadosChat.Texto)
			}

		case "ERRO":
			var e protocolo.DadosErro
			if err := json.Unmarshal(msg.Dados, &e); err == nil {
				fmt.Printf("\n[ERRO] %s\n> ", e.Mensagem)
			}
		}
	}
}

func main() {
	fmt.Println("--- Jogo de Cartas Multiplayer ---")
	scanner := bufio.NewScanner(os.Stdin)

	fmt.Print("Digite seu nome de usuário: ")
	scanner.Scan()
	meuNome = scanner.Text()
	if strings.TrimSpace(meuNome) == "" {
		meuNome = "Jogador"
	}

	// ajuste o host conforme seu cenário (ex.: "127.0.0.1:65432")
	conn, err := net.Dial("tcp", "servidor:65432")
	if err != nil {
		fmt.Printf("Não foi possível conectar: %s\n", err)
		return
	}
	defer conn.Close()
	fmt.Printf("Conectado como '%s'. Aguardando pareamento...\n", meuNome)

	encoder := json.NewEncoder(conn)

	// login e fila automática
	_ = encoder.Encode(protocolo.Mensagem{Comando: "LOGIN", Dados: mustJSON(protocolo.DadosLogin{Nome: meuNome})})
	_ = encoder.Encode(protocolo.Mensagem{Comando: "ENTRAR_NA_FILA"})

	go lerServidor(conn)

	// loop de entrada
	for {
		fmt.Print("> ")
		if !scanner.Scan() {
			return
		}
		entrada := scanner.Text()
		partes := strings.Fields(entrada)
		if len(partes) == 0 {
			continue
		}

		switch partes[0] {
		case "/jogar":
			if len(partes) < 2 {
				fmt.Println("[SISTEMA] Uso: /jogar <nome da carta>")
				continue
			}
			// aceita nome da carta com espaços após o comando
			cartaNome := strings.TrimSpace(entrada[len("/jogar"):])
			_ = encoder.Encode(protocolo.Mensagem{
				Comando: "JOGAR_CARTA",
				Dados:   mustJSON(protocolo.DadosJogarCarta{CartaID: cartaNome}),
			})

		case "/buy_pack":
			// compra 1 pacote (= 10 cartas)
			_ = encoder.Encode(protocolo.Mensagem{
				Comando: "COMPRAR_PACOTE",
				Dados:   mustJSON(protocolo.ComprarPacoteReq{Quantidade: 1}),
			})

		default:
			// qualquer texto envia pro chat
			_ = encoder.Encode(protocolo.Mensagem{
				Comando: "ENVIAR_CHAT",
				Dados:   mustJSON(protocolo.DadosEnviarChat{Texto: entrada}),
			})
		}
	}
}

func mustJSON(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}
