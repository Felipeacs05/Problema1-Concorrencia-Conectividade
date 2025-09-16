package main

// ===================== BAREMA ITEM 1: ARQUITETURA =====================
// Este é o componente cliente do jogo de cartas multiplayer.
// O cliente gerencia: interface do usuário, comunicação com o servidor,
// exibição do estado do jogo e processamento de comandos do usuário.

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

// BAREMA ITEM 1: ARQUITETURA - Variáveis globais do cliente
var meuNome string                  // Nome do jogador atual
var meuInventario []protocolo.Carta // Cartas que o jogador possui

// BAREMA ITEM 1: ARQUITETURA - Exibe a interface de ajuda para o usuário
// Mostra todos os comandos disponíveis e suas funcionalidades
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

// BAREMA ITEM 2: COMUNICAÇÃO - Processa mensagens recebidas do servidor
// Roda em uma goroutine separada para não bloquear a interface do usuário
func handleServerMessages(conn net.Conn) {
	decoder := json.NewDecoder(conn)
	for {
		var msg protocolo.Mensagem
		if err := decoder.Decode(&msg); err != nil {
			fmt.Println("\n[CLIENTE] Conexão com o servidor foi perdida.")
			os.Exit(0)
		}

		// BAREMA ITEM 3: API REMOTA - Processa diferentes tipos de mensagens do servidor
		switch msg.Comando {

		// BAREMA ITEM 6: LATÊNCIA - Processa resposta de ping para medir latência
		case "PONG":
			var dadosPong protocolo.DadosPong
			if err := json.Unmarshal(msg.Dados, &dadosPong); err == nil {
				latencia := time.Now().UnixMilli() - dadosPong.Timestamp
				fmt.Printf("\r[SISTEMA] Sua latência com o servidor é de %dms.\n> ", latencia)
			}

		// BAREMA ITEM 7: PARTIDAS - Notifica que uma partida foi encontrada
		case "PARTIDA_ENCONTRADA":
			var dados protocolo.DadosPartidaEncontrada
			if err := json.Unmarshal(msg.Dados, &dados); err == nil {
				fmt.Printf("\r[SISTEMA] Partida encontrada! Seu oponente é: %s.\n", dados.OponenteNome)
				printAjuda()
			}

		// BAREMA ITEM 3: API REMOTA - Atualiza o estado do jogo na interface
		case "ATUALIZACAO_JOGO":
			var dados protocolo.DadosAtualizacaoJogo
			if err := json.Unmarshal(msg.Dados, &dados); err == nil {
				fmt.Printf("\r--- Rodada %d ---\n", dados.NumeroRodada)
				fmt.Println(dados.MensagemDoTurno)

				// Exibe cartas jogadas na mesa
				if len(dados.UltimaJogada) > 0 {
					fmt.Println("\nCartas na mesa:")
					for nome, carta := range dados.UltimaJogada {
						fmt.Printf("  %s: %s %s (Poder: %d)\n", nome, carta.Nome, carta.Naipe, carta.Valor)
					}
				}

				// Exibe vencedor da jogada atual
				if dados.VencedorJogada != "" && dados.VencedorJogada != "EMPATE" {
					fmt.Printf("\nVencedor da jogada: %s\n", dados.VencedorJogada)
				}

				// Exibe vencedor da rodada
				if dados.VencedorRodada != "" && dados.VencedorRodada != "EMPATE" {
					fmt.Printf("Vencedor da rodada %d: %s\n", dados.NumeroRodada, dados.VencedorRodada)
				}

				// Exibe contagem de cartas restantes
				fmt.Println("\nCartas restantes:")
				for nome, contagem := range dados.ContagemCartas {
					fmt.Printf("  %s: %d cartas\n", nome, contagem)
				}
				fmt.Println("-------------------")
				fmt.Print("> ")
			}

		// BAREMA ITEM 7: PARTIDAS - Notifica fim da partida
		case "FIM_DE_JOGO":
			var dados protocolo.DadosFimDeJogo
			if err := json.Unmarshal(msg.Dados, &dados); err == nil {
				if dados.VencedorNome == "EMPATE" {
					fmt.Printf("\n=== FIM DE JOGO — EMPATE ===\n")
				} else {
					fmt.Printf("\n=== FIM DE JOGO — VENCEDOR DA PARTIDA: %s ===\n", dados.VencedorNome)
				}
				// Volta ao lobby: pode comprar pacotes e reiniciar
				printAjuda()
			}

		// BAREMA ITEM 8: PACOTES - Processa resultado da compra de pacote
		case "PACOTE_RESULTADO":
			var r protocolo.ComprarPacoteResp
			if err := json.Unmarshal(msg.Dados, &r); err == nil {
				// Limpa o inventário antigo ao comprar um novo pacote
				meuInventario = r.Cartas

				// Exibe as cartas recebidas de forma organizada
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

// BAREMA ITEM 2: COMUNICAÇÃO - Função principal do cliente
// Inicializa conexão com o servidor e gerencia interface do usuário
func main() {
	fmt.Println("--- Jogo de Cartas Multiplayer ---")
	scanner := bufio.NewScanner(os.Stdin)

	// BAREMA ITEM 1: ARQUITETURA - Solicita nome do usuário
	fmt.Print("Digite seu nome de usuário: ")
	scanner.Scan()
	meuNome = scanner.Text()
	if strings.TrimSpace(meuNome) == "" {
		meuNome = "Jogador" // Nome padrão se não informado
	}

	// BAREMA ITEM 2: COMUNICAÇÃO - Conecta com o servidor
	// Ajuste o host conforme seu cenário (ex.: "127.0.0.1:65432")
	conn, err := net.Dial("tcp", "servidor:65432")
	if err != nil {
		fmt.Printf("Não foi possível conectar: %s\n", err)
		return
	}
	defer conn.Close()
	fmt.Printf("Conectado como '%s'. Aguardando pareamento...\n", meuNome)

	encoder := json.NewEncoder(conn)

	// BAREMA ITEM 3: API REMOTA - Login e entrada na fila automática
	_ = encoder.Encode(protocolo.Mensagem{Comando: "LOGIN", Dados: mustJSON(protocolo.DadosLogin{Nome: meuNome})})
	_ = encoder.Encode(protocolo.Mensagem{Comando: "ENTRAR_NA_FILA"})

	// BAREMA ITEM 2: COMUNICAÇÃO - Inicia goroutine para processar mensagens do servidor
	go handleServerMessages(conn)

	// BAREMA ITEM 1: ARQUITETURA - Loop principal de interface do usuário
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

		// BAREMA ITEM 3: API REMOTA - Processa comandos do usuário
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
			// BAREMA ITEM 6: LATÊNCIA - Usa ICMP para medição mais precisa de latência
			medirLatenciaICMP()

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

// BAREMA ITEM 6: LATÊNCIA - Mede latência usando ICMP para máxima precisão
func medirLatenciaICMP() {
	// BAREMA ITEM 2: COMUNICAÇÃO - Cria socket ICMP raw
	conn, err := net.Dial("ip4:icmp", "servidor")
	if err != nil {
		fmt.Printf("[ERRO] Não foi possível conectar via ICMP: %v\n", err)
		return
	}
	defer conn.Close()

	// BAREMA ITEM 6: LATÊNCIA - Cria pacote ICMP Echo Request
	icmpPacket := createICMPEchoRequest()

	start := time.Now()
	_, err = conn.Write(icmpPacket)
	if err != nil {
		fmt.Printf("[ERRO] Falha ao enviar ping ICMP: %v\n", err)
		return
	}

	// BAREMA ITEM 6: LATÊNCIA - Configura timeout para resposta
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))

	// BAREMA ITEM 6: LATÊNCIA - Aguarda resposta
	buffer := make([]byte, 1024)
	_, err = conn.Read(buffer)
	if err != nil {
		fmt.Printf("[ERRO] Timeout ou erro ao receber resposta ICMP: %v\n", err)
		return
	}

	// BAREMA ITEM 6: LATÊNCIA - Calcula latência total
	latencia := time.Since(start)
	fmt.Printf("\r[SISTEMA] Sua latência ICMP com o servidor é de %.2fms.\n> ", float64(latencia.Nanoseconds())/1e6)
}

// BAREMA ITEM 6: LATÊNCIA - Cria pacote ICMP Echo Request
func createICMPEchoRequest() []byte {
	// BAREMA ITEM 6: LATÊNCIA - Estrutura do pacote ICMP
	packet := make([]byte, 8)
	packet[0] = 8 // Tipo: Echo Request
	packet[1] = 0 // Código: 0
	packet[2] = 0 // Checksum (será calculado)
	packet[3] = 0 // Checksum (será calculado)
	packet[4] = 0 // Identifier (16 bits)
	packet[5] = 1 // Identifier (16 bits)
	packet[6] = 0 // Sequence Number (16 bits)
	packet[7] = 1 // Sequence Number (16 bits)

	// BAREMA ITEM 6: LATÊNCIA - Calcula checksum
	checksum := calculateICMPChecksum(packet)
	packet[2] = byte(checksum >> 8)
	packet[3] = byte(checksum & 0xFF)

	return packet
}

// BAREMA ITEM 6: LATÊNCIA - Calcula checksum ICMP
func calculateICMPChecksum(data []byte) uint16 {
	var sum uint32
	for i := 0; i < len(data); i += 2 {
		if i+1 < len(data) {
			sum += uint32(data[i])<<8 + uint32(data[i+1])
		} else {
			sum += uint32(data[i]) << 8
		}
	}

	for sum>>16 != 0 {
		sum = (sum & 0xFFFF) + (sum >> 16)
	}

	return uint16(^sum)
}

func mustJSON(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}
