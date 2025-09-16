package main

// ===================== BAREMA ITEM 9: TESTES =====================
// Este arquivo implementa testes de estresse automatizados para o jogo de cartas.
// Simula múltiplos jogadores simultâneos para testar concorrência, performance
// e justiça na distribuição de recursos.

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"meujogo/protocolo"
	"net"
	"sort"
	"sync"
	"time"
)

// BAREMA ITEM 9: TESTES - Configurações do teste de estresse
const (
	numBots        = 10000            // Número de bots simultâneos para testar concorrência
	testDuration   = 90 * time.Second // Duração total do teste
	rampUpDuration = 30 * time.Second // Tempo para iniciar todos os bots gradualmente
	serverAddr     = "servidor:65432" // Endereço do servidor para conectar
)

// BAREMA ITEM 9: TESTES - Estrutura para armazenar resultados do teste
// Coleta estatísticas de performance, concorrência e justiça
type TestReport struct {
	totalBots            int             // Total de bots que tentaram conectar
	connectionsSucceeded int             // Conexões bem-sucedidas
	purchasesSucceeded   int             // Compras de pacotes bem-sucedidas
	gamesCompleted       int             // Partidas completadas
	totalErrors          int             // Total de erros encontrados
	latencies            []time.Duration // Medições de latência coletadas
	mu                   sync.Mutex      // BAREMA ITEM 5: CONCORRÊNCIA - Protege acesso concorrente aos dados
}

// BAREMA ITEM 9: TESTES - Estrutura que representa um bot de teste
// Simula um jogador real conectado ao servidor
type Bot struct {
	ID         int               // Identificador único do bot
	Conn       net.Conn          // Conexão TCP com o servidor
	Encoder    *json.Encoder     // Codificador JSON para envio
	Decoder    *json.Decoder     // Decodificador JSON para recebimento
	Nome       string            // Nome do bot
	Inventario []protocolo.Carta // Cartas que o bot possui
	pingStart  time.Time         // BAREMA ITEM 6: LATÊNCIA - Timestamp para medição de ping
	mu         sync.Mutex        // BAREMA ITEM 5: CONCORRÊNCIA - Protege o inventário
}

// BAREMA ITEM 9: TESTES - Simula o ciclo de vida completo de um bot
// Conecta, joga partidas e coleta métricas de performance
func runBotLifecycle(ctx context.Context, botID int, report *TestReport, wg *sync.WaitGroup) {
	defer wg.Done()

	// BAREMA ITEM 2: COMUNICAÇÃO - Tenta conectar com timeout e retry
	var conn net.Conn
	var err error
	maxRetries := 3
	for i := 0; i < maxRetries; i++ {
		conn, err = net.DialTimeout("tcp", serverAddr, 5*time.Second)
		if err == nil {
			break
		}
		if i < maxRetries-1 {
			time.Sleep(time.Duration(i+1) * time.Second)
		}
	}
	if err != nil {
		log.Printf("❌ Bot %d: Falha ao conectar após %d tentativas: %v", botID, maxRetries, err)
		report.mu.Lock()
		report.totalErrors++
		report.mu.Unlock()
		return
	}
	defer conn.Close()

	// BAREMA ITEM 9: TESTES - Registra conexão bem-sucedida
	report.mu.Lock()
	report.connectionsSucceeded++
	report.mu.Unlock()

	bot := &Bot{
		ID:      botID,
		Conn:    conn,
		Encoder: json.NewEncoder(conn),
		Decoder: json.NewDecoder(conn),
		Nome:    fmt.Sprintf("Bot-%d", botID),
	}

	// BAREMA ITEM 2: COMUNICAÇÃO - Canal para mensagens assíncronas
	incomingMessages := make(chan protocolo.Mensagem, 10)
	errChan := make(chan error, 1)
	go readFromServer(bot, incomingMessages, errChan)

	// BAREMA ITEM 3: API REMOTA - Login e entrada na fila
	enviarComando(bot, "LOGIN", protocolo.DadosLogin{Nome: bot.Nome})
	enviarComando(bot, "ENTRAR_NA_FILA", nil)

	// BAREMA ITEM 6: LATÊNCIA - Ticker para medições de ping (reduzido para evitar sobrecarga)
	pingTicker := time.NewTicker(10 * time.Second)
	defer pingTicker.Stop()

	// BAREMA ITEM 9: TESTES - Loop principal do bot
	for {
		select {
		case msg := <-incomingMessages:
			// BAREMA ITEM 3: API REMOTA - Processa mensagens do servidor
			switch msg.Comando {
			case "PARTIDA_ENCONTRADA":
				// BAREMA ITEM 8: PACOTES - Compra pacote quando encontra partida
				enviarComando(bot, "COMPRAR_PACOTE", protocolo.ComprarPacoteReq{Quantidade: 1})
			case "PACOTE_RESULTADO":
				var resp protocolo.ComprarPacoteResp
				if json.Unmarshal(msg.Dados, &resp) == nil {
					// BAREMA ITEM 9: TESTES - Registra compra bem-sucedida
					report.mu.Lock()
					report.purchasesSucceeded++
					report.mu.Unlock()
					bot.mu.Lock()
					bot.Inventario = resp.Cartas
					bot.mu.Unlock()
					// BAREMA ITEM 7: PARTIDAS - Joga primeira carta para iniciar partida
					jogarPrimeiraCarta(bot)
				}
			case "ATUALIZACAO_JOGO":
				var dados protocolo.DadosAtualizacaoJogo
				if json.Unmarshal(msg.Dados, &dados) == nil {
					// BAREMA ITEM 7: PARTIDAS - Joga próxima carta quando jogada anterior é resolvida
					if dados.VencedorJogada != "" {
						jogarPrimeiraCarta(bot)
					}
				}
			case "FIM_DE_JOGO":
				// BAREMA ITEM 9: TESTES - Registra partida completada
				report.mu.Lock()
				report.gamesCompleted++
				report.mu.Unlock()
				// BAREMA ITEM 7: PARTIDAS - Volta para fila para nova partida
				enviarComando(bot, "ENTRAR_NA_FILA", nil)
			case "PONG":
				// BAREMA ITEM 6: LATÊNCIA - Calcula e armazena latência
				if !bot.pingStart.IsZero() {
					latencia := time.Since(bot.pingStart)
					report.mu.Lock()
					report.latencies = append(report.latencies, latencia)
					report.mu.Unlock()
				}
			case "PING":
				// BAREMA ITEM 6: LATÊNCIA - Responde ping do servidor
				var dadosPing protocolo.DadosPing
				if json.Unmarshal(msg.Dados, &dadosPing) == nil {
					enviarComando(bot, "PONG", protocolo.DadosPong{Timestamp: dadosPing.Timestamp})
				}
			}
		case <-pingTicker.C:
			// BAREMA ITEM 6: LATÊNCIA - Mede latência via ICMP com delay aleatório
			// BAREMA ITEM 5: CONCORRÊNCIA - Delay aleatório para evitar contenção simultânea
			go func() {
				delay := time.Duration(bot.ID%1000) * time.Millisecond
				time.Sleep(delay)
				bot.pingStart = time.Now()
				medirLatenciaICMPBot(bot, report)
			}()
		case err := <-errChan:
			// BAREMA ITEM 9: TESTES - Trata erros de comunicação
			if err.Error() != "EOF" {
				log.Printf("❌ Bot %d: Erro de comunicação: %v. Saindo.", bot.ID, err)
				report.mu.Lock()
				report.totalErrors++
				report.mu.Unlock()
			}
			return
		case <-ctx.Done():
			// BAREMA ITEM 9: TESTES - Encerra bot quando teste termina
			enviarComando(bot, "QUIT", nil)
			return
		}
	}
}

// readFromServer é uma goroutine que apenas lê do socket e envia para um canal.
func readFromServer(b *Bot, incoming chan<- protocolo.Mensagem, errs chan<- error) {
	for {
		var msg protocolo.Mensagem
		if err := b.Decoder.Decode(&msg); err != nil {
			errs <- err
			return
		}
		incoming <- msg
	}
}

func jogarPrimeiraCarta(b *Bot) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.Inventario) > 0 {
		cartaAJogar := b.Inventario[0]
		b.Inventario = b.Inventario[1:] // Remove a carta da mão
		enviarComando(b, "JOGAR_CARTA", protocolo.DadosJogarCarta{CartaID: cartaAJogar.ID})
	}
}

// BAREMA ITEM 6: LATÊNCIA - Mede latência ICMP para bots de teste
func medirLatenciaICMPBot(bot *Bot, report *TestReport) {
	// BAREMA ITEM 6: LATÊNCIA - Timeout de conexão para evitar bloqueios
	conn, err := net.DialTimeout("ip4:icmp", "servidor", 1*time.Second)
	if err != nil {
		return
	}
	defer conn.Close()

	// BAREMA ITEM 6: LATÊNCIA - Cria pacote ICMP Echo Request
	icmpPacket := createICMPEchoRequestBot()

	start := time.Now()
	conn.Write(icmpPacket)

	// BAREMA ITEM 6: LATÊNCIA - Timeout reduzido para evitar latências altas
	conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
	buffer := make([]byte, 1024)
	_, err = conn.Read(buffer)
	if err != nil {
		return
	}

	latencia := time.Since(start)

	// BAREMA ITEM 6: LATÊNCIA - Filtra latências muito altas (provavelmente timeouts)
	if latencia > 1*time.Second {
		return
	}

	report.mu.Lock()
	report.latencies = append(report.latencies, latencia)
	report.mu.Unlock()
}

// BAREMA ITEM 6: LATÊNCIA - Cria pacote ICMP Echo Request para bots
func createICMPEchoRequestBot() []byte {
	packet := make([]byte, 8)
	packet[0] = 8 // Tipo: Echo Request
	packet[1] = 0 // Código: 0
	packet[2] = 0 // Checksum (será calculado)
	packet[3] = 0 // Checksum (será calculado)
	packet[4] = 0 // Identifier (16 bits)
	packet[5] = 1 // Identifier (16 bits)
	packet[6] = 0 // Sequence Number (16 bits)
	packet[7] = 1 // Sequence Number (16 bits)

	checksum := calculateICMPChecksumBot(packet)
	packet[2] = byte(checksum >> 8)
	packet[3] = byte(checksum & 0xFF)

	return packet
}

// BAREMA ITEM 6: LATÊNCIA - Calcula checksum ICMP para bots
func calculateICMPChecksumBot(data []byte) uint16 {
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

func enviarComando(b *Bot, comando string, dados interface{}) {
	var rawDados json.RawMessage
	if dados != nil {
		jsonBytes, _ := json.Marshal(dados)
		rawDados = jsonBytes
	}
	msg := protocolo.Mensagem{Comando: comando, Dados: rawDados}
	b.Encoder.Encode(msg)
}

func main() {
	log.Printf("Iniciando teste de estresse com %d bots por %v (aquecimento de %v)...", numBots, testDuration, rampUpDuration)

	// BAREMA ITEM 9: TESTES - Aguarda servidor estar pronto antes de iniciar bots
	log.Println("Aguardando servidor estar disponível...")
	time.Sleep(5 * time.Second)

	report := &TestReport{totalBots: numBots}
	var wg sync.WaitGroup
	var printOnce sync.Once // Garante que o relatório seja impresso apenas uma vez.

	ctx, cancel := context.WithTimeout(context.Background(), testDuration)
	defer cancel()

	// Calcula o intervalo entre o início de cada bot para distribuí-los ao longo do ramp-up.
	intervalo := time.Duration(0)
	if numBots > 0 {
		intervalo = rampUpDuration / time.Duration(numBots)
	}

	for i := 0; i < numBots; i++ {
		wg.Add(1)
		go runBotLifecycle(ctx, i, report, &wg)
		time.Sleep(intervalo) // Espera o intervalo calculado antes de iniciar o próximo bot.
	}

	log.Println("Todos os bots foram iniciados. Teste em andamento...")

	// Goroutine para esperar o fim do teste e imprimir o relatório.
	go func() {
		wg.Wait()
		printOnce.Do(func() { printReport(report) })
	}()

	<-ctx.Done()
	log.Println("Tempo do teste esgotado. Aguardando bots finalizarem...")
	// Espera um pouco mais para os bots receberem o sinal de ctx.Done() e saírem.
	time.Sleep(2 * time.Second)
	// BAREMA ITEM 9: TESTES - Relatório já será impresso pela goroutine acima
}

func printReport(r *TestReport) {
	fmt.Println("\n======================================")
	fmt.Println("    Relatório Final do Teste de Estresse")
	fmt.Println("======================================")
	fmt.Printf("Duração do Teste:        %v\n", testDuration)
	fmt.Printf("Total de Bots:           %d\n", r.totalBots)
	fmt.Println("--------------------------------------")
	fmt.Printf("Conexões bem-sucedidas:  %d / %d\n", r.connectionsSucceeded, r.totalBots)
	fmt.Printf("Total de Compras:          %d\n", r.purchasesSucceeded)
	fmt.Printf("Partidas concluídas:       %d\n", r.gamesCompleted)
	fmt.Printf("Total de Erros:            %d\n", r.totalErrors)
	fmt.Println("--------------------------------------")

	if len(r.latencies) > 0 {
		sort.Slice(r.latencies, func(i, j int) bool {
			return r.latencies[i] < r.latencies[j]
		})

		var totalLatencia time.Duration
		for _, l := range r.latencies {
			totalLatencia += l
		}

		minLat := r.latencies[0]
		maxLat := r.latencies[len(r.latencies)-1]
		avgLat := totalLatencia / time.Duration(len(r.latencies))
		p95Index := int(float64(len(r.latencies)) * 0.95)
		if p95Index >= len(r.latencies) {
			p95Index = len(r.latencies) - 1
		}
		if p95Index < 0 {
			p95Index = 0
		}
		p95Lat := r.latencies[p95Index]

		fmt.Println("Estatísticas de Latência (ms):")
		fmt.Printf("  Mínima: %.3fms\n", float64(minLat.Microseconds())/1000.0)
		fmt.Printf("  Média:  %.3fms\n", float64(avgLat.Microseconds())/1000.0)
		fmt.Printf("  Máxima: %.3fms\n", float64(maxLat.Microseconds())/1000.0)
		fmt.Printf("  P95:    %.3fms\n", float64(p95Lat.Microseconds())/1000.0)
	} else {
		fmt.Println("Nenhuma medição de latência registrada.")
	}

	fmt.Println("======================================")
}
