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

func printAjuda() {
	fmt.Println("\n------------ COMANDOS ---------------")
	fmt.Println("/buy_pack       - compra 10 cartas (apenas fora da partida)")
	fmt.Println("/jogar          - no lobby: inicia; em jogo: joga sua próxima carta")
	fmt.Println("/chat <mensagem> - envia mensagem (mostra [VOCÊ] se for você)")
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
				fmt.Println("\n--- Status da Rodada ---")
				fmt.Println(dados.MensagemDoTurno)
				if len(dados.UltimaJogada) > 0 {
					fmt.Println("Cartas na mesa:")
					for nome, carta := range dados.UltimaJogada {
						fmt.Printf("  - %s jogou: %s (v=%d)\n", nome, carta.Nome, carta.Valor)
					}
				}
				if dados.VencedorRodada == "EMPATE" {
					fmt.Println("Resultado da rodada: EMPATE")
				} else if dados.VencedorRodada != "" {
					fmt.Printf("Resultado da rodada: VENCEDOR: %s\n", dados.VencedorRodada)
				}
				fmt.Println("Cartas restantes:")
				for nome, contagem := range dados.ContagemCartas {
					fmt.Printf("  - %s: %d cartas\n", nome, contagem)
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
					fmt.Printf("\n=== FIM DE JOGO — VENCEDOR: %s ===\n", dados.VencedorNome)
				}
				// volta ao lobby: pode comprar pacotes e reiniciar
				printAjuda()
			}

		case "PACOTE_RESULTADO":
			var r protocolo.ComprarPacoteResp
			if err := json.Unmarshal(msg.Dados, &r); err == nil {
				n := make([]string, 0, len(r.Cartas))
				for _, c := range r.Cartas {
					n = append(n, fmt.Sprintf("%s(%d,%s)", c.Nome, c.Valor, c.Raridade))
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
			_ = encoder.Encode(protocolo.Mensagem{Comando: "JOGAR_CARTA"})

		case "/buy_pack":
			// compra 1 pacote (= 10 cartas)
			_ = encoder.Encode(protocolo.Mensagem{
				Comando: "COMPRAR_PACOTE",
				Dados:   mustJSON(protocolo.ComprarPacoteReq{Quantidade: 1}),
			})

		case "/chat":
			if len(partes) < 2 {
				fmt.Println("[SISTEMA] Uso: /chat <sua mensagem>")
				continue
			}
			txt := strings.TrimSpace(strings.TrimPrefix(entrada, "/chat"))
			_ = encoder.Encode(protocolo.Mensagem{
				Comando: "ENVIAR_CHAT",
				Dados:   mustJSON(protocolo.DadosEnviarChat{Texto: txt}),
			})

		default:
			// atalho: qualquer texto envia pro chat
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
