package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"meujogo/protocolo"
	"net"
	"os"
	"strings"
	"time"
)

var meuNome string
var meuInventario []protocolo.Carta

func printAjuda() {
	fmt.Println("\n------------ COMANDOS ---------------")
	fmt.Println("/comprar    - Compra um pacote de cartas para (re)iniciar a partida.")
	fmt.Println("/jogar <ID> - Joga uma carta da sua mão usando o ID dela.")
	fmt.Println("/cartas     - Mostra as cartas que você tem na mão.")
	fmt.Println("/ping       - Mede sua latência (atraso) com o servidor.")
	fmt.Println("/sair       - Abandona a partida atual e volta para a fila.")
	fmt.Println("Qualquer outra coisa que você digitar será enviada como chat.")
	fmt.Println("-------------------------------------")
	fmt.Print("> ")
}

func handleServerMessages(conn net.Conn) {
	decoder := json.NewDecoder(conn)
	for {
		var msg protocolo.Mensagem
		if err := decoder.Decode(&msg); err != nil {
			fmt.Println("\n[CLIENTE] Conexão com o servidor foi perdida.")
			os.Exit(0)
		}

		switch msg.Comando {

		case "PONG":
			var dadosPong protocolo.DadosPong
			if err := json.Unmarshal(msg.Dados, &dadosPong); err == nil {
				latencia := time.Now().UnixMilli() - dadosPong.Timestamp
				fmt.Printf("\r[SISTEMA] Sua latência com o servidor é de %dms.\n> ", latencia)
			}

		case "PARTIDA_ENCONTRADA":
			var dados protocolo.DadosPartidaEncontrada
			if err := json.Unmarshal(msg.Dados, &dados); err == nil {
				fmt.Printf("\r[SISTEMA] Partida encontrada! Seu oponente é: %s.\n", dados.OponenteNome)
				printAjuda()
			}

		case "ATUALIZACAO_JOGO":
			var dados protocolo.DadosAtualizacaoJogo
			if err := json.Unmarshal(msg.Dados, &dados); err == nil {
				fmt.Printf("\r--- Rodada %d ---\n", dados.NumeroRodada)
				fmt.Println(dados.MensagemDoTurno)
				if len(dados.UltimaJogada) > 0 {
					fmt.Println("\nCartas na mesa:")
					for nome, carta := range dados.UltimaJogada {
						fmt.Printf("  %s: %s %s (Poder: %d)\n", nome, carta.Nome, carta.Naipe, carta.Valor)
					}
				}
				if dados.VencedorJogada != "" && dados.VencedorJogada != "EMPATE" {
					fmt.Printf("\nVencedor da jogada: %s\n", dados.VencedorJogada)
				}
				if dados.VencedorRodada != "" && dados.VencedorRodada != "EMPATE" {
					fmt.Printf("Vencedor da rodada %d: %s\n", dados.NumeroRodada, dados.VencedorRodada)
				}
				fmt.Println("\nCartas restantes:")
				for nome, contagem := range dados.ContagemCartas {
					fmt.Printf("  %s: %d cartas\n", nome, contagem)
				}
				fmt.Println("-------------------")
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
				// Limpa o inventário antigo ao comprar um novo pacote
				meuInventario = r.Cartas

				n := make([]string, 0, len(r.Cartas))
				for _, c := range r.Cartas {
					n = append(n, fmt.Sprintf("%s (ID: %s, Poder: %d)", c.Nome, c.ID, c.Valor))
				}
				fmt.Printf("\n[PACOTE] Você recebeu: %s\n", strings.Join(n, ", "))
				fmt.Print("> ")
			}

		case "CARTAS_DETALHADAS":
			var e protocolo.DadosErro
			if err := json.Unmarshal(msg.Dados, &e); err == nil {
				fmt.Printf("\r%s\n> ", e.Mensagem)
			}

		case "SISTEMA":
			var e protocolo.DadosErro
			if err := json.Unmarshal(msg.Dados, &e); err == nil {
				fmt.Printf("\r%s\n> ", e.Mensagem)
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

		case "PING":
			var dadosPing protocolo.DadosPing
			if err := json.Unmarshal(msg.Dados, &dadosPing); err == nil {
				// Responde ao ping do servidor para manter a conexão ativa
				encoder := json.NewEncoder(conn)
				_ = encoder.Encode(protocolo.Mensagem{
					Comando: "PONG",
					Dados:   mustJSON(protocolo.DadosPong{Timestamp: dadosPing.Timestamp}),
				})
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

	go handleServerMessages(conn)

	// loop de entrada
	printAjuda()
	for scanner.Scan() {
		entrada := scanner.Text()
		partes := strings.Fields(entrada)
		if len(partes) == 0 {
			fmt.Print("> ")
			continue
		}

		comando := partes[0]
		var msg protocolo.Mensagem

		switch comando {
		case "/comprar":
			msg = protocolo.Mensagem{
				Comando: "COMPRAR_PACOTE",
				Dados:   mustJSON(protocolo.ComprarPacoteReq{Quantidade: 1}),
			}

		case "/jogar":
			if len(partes) < 2 {
				fmt.Println("[SISTEMA] Uso: /jogar <ID_da_carta>")
				fmt.Print("> ")
				continue
			}
			cartaID := partes[1]
			msg = protocolo.Mensagem{
				Comando: "JOGAR_CARTA",
				Dados:   mustJSON(protocolo.DadosJogarCarta{CartaID: cartaID}),
			}

		case "/cartas":
			msg = protocolo.Mensagem{Comando: "VER_CARTAS"}

		case "/ping":
			msg = protocolo.Mensagem{
				Comando: "PING",
				Dados:   mustJSON(protocolo.DadosPing{Timestamp: time.Now().UnixMilli()}),
			}

		case "/sair":
			msg = protocolo.Mensagem{Comando: "SAIR_DA_SALA"}
			fmt.Println("[SISTEMA] Você saiu da sala. Aguardando novo oponente...")

		default:
			// qualquer texto que não seja comando vira chat
			msg = protocolo.Mensagem{
				Comando: "ENVIAR_CHAT",
				Dados:   mustJSON(protocolo.DadosEnviarChat{Texto: entrada}),
			}
		}

		if err := encoder.Encode(msg); err != nil {
			fmt.Println("[CLIENTE] Falha ao enviar mensagem para o servidor.")
			return
		}
		fmt.Print("> ")
	}
}

func mustJSON(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}
