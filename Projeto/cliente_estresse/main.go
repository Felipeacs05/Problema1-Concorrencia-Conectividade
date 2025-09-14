package main

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

// --- Configurações do Teste ---
const (
	numBots        = 10000            // Altere o número de jogadores simultâneos aqui
	testDuration   = 90 * time.Second // Duração total do teste
	rampUpDuration = 30 * time.Second // Tempo para iniciar todos os bots gradualmente
	serverAddr     = "servidor:65432"
)

// TestReport armazena os resultados consolidados do teste.
type TestReport struct {
	totalBots            int
	connectionsSucceeded int
	purchasesSucceeded   int
	gamesCompleted       int
	totalErrors          int
	latencies            []time.Duration
	mu                   sync.Mutex
}

// Bot representa um cliente de teste.
type Bot struct {
	ID         int
	Conn       net.Conn
	Encoder    *json.Encoder
	Decoder    *json.Decoder
	Nome       string
	Inventario []protocolo.Carta
	pingStart  time.Time
	mu         sync.Mutex // Protege o inventário
}

// runBotLifecycle agora simula partidas completas e loga latência em tempo real.
func runBotLifecycle(ctx context.Context, botID int, report *TestReport, wg *sync.WaitGroup) {
	defer wg.Done()

	conn, err := net.DialTimeout("tcp", serverAddr, 10*time.Second)
	if err != nil {
		log.Printf("❌ Bot %d: Falha ao conectar: %v", botID, err)
		report.mu.Lock()
		report.totalErrors++
		report.mu.Unlock()
		return
	}
	defer conn.Close()

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

	incomingMessages := make(chan protocolo.Mensagem, 10)
	errChan := make(chan error, 1)
	go readFromServer(bot, incomingMessages, errChan)

	enviarComando(bot, "LOGIN", protocolo.DadosLogin{Nome: bot.Nome})
	enviarComando(bot, "ENTRAR_NA_FILA", nil)

	pingTicker := time.NewTicker(5 * time.Second)
	defer pingTicker.Stop()

	for {
		select {
		case msg := <-incomingMessages:
			switch msg.Comando {
			case "PARTIDA_ENCONTRADA":
				enviarComando(bot, "COMPRAR_PACOTE", protocolo.ComprarPacoteReq{Quantidade: 1})
			case "PACOTE_RESULTADO":
				var resp protocolo.ComprarPacoteResp
				if json.Unmarshal(msg.Dados, &resp) == nil {
					report.mu.Lock()
					report.purchasesSucceeded++
					report.mu.Unlock()
					bot.mu.Lock()
					bot.Inventario = resp.Cartas
					bot.mu.Unlock()
					// Joga a primeira carta imediatamente para iniciar a partida.
					jogarPrimeiraCarta(bot)
				}
			case "ATUALIZACAO_JOGO":
				var dados protocolo.DadosAtualizacaoJogo
				if json.Unmarshal(msg.Dados, &dados) == nil {
					// Apenas joga a próxima carta se a jogada anterior foi resolvida
					// (indicado pela presença de um vencedor da jogada).
					if dados.VencedorJogada != "" {
						jogarPrimeiraCarta(bot)
					}
				}
			case "FIM_DE_JOGO":
				// Fase 3: Partida Concluída
				report.mu.Lock()
				report.gamesCompleted++
				report.mu.Unlock()
				enviarComando(bot, "ENTRAR_NA_FILA", nil)
			case "PONG":
				if !bot.pingStart.IsZero() {
					latencia := time.Since(bot.pingStart)
					// A latência é armazenada para o relatório final, mas não é logada em tempo real.
					report.mu.Lock()
					report.latencies = append(report.latencies, latencia)
					report.mu.Unlock()
				}
			case "PING": // Adicionado para responder aos pings do servidor
				var dadosPing protocolo.DadosPing
				if json.Unmarshal(msg.Dados, &dadosPing) == nil {
					enviarComando(bot, "PONG", protocolo.DadosPong{Timestamp: dadosPing.Timestamp})
				}
			}
		case <-pingTicker.C:
			bot.pingStart = time.Now()
			enviarComando(bot, "PING", protocolo.DadosPing{Timestamp: time.Now().UnixMilli()})
		case err := <-errChan:
			// Não loga EOF como erro, pois é esperado.
			if err.Error() != "EOF" {
				log.Printf("❌ Bot %d: Erro de comunicação: %v. Saindo.", bot.ID, err)
				report.mu.Lock()
				report.totalErrors++
				report.mu.Unlock()
			}
			return
		case <-ctx.Done():
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
	log.Printf("Iniciando teszzzte de estresse com %d bots por %v (aquecimento de %v)...", numBots, testDuration, rampUpDuration)

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
	printOnce.Do(func() { printReport(report) }) // Imprime caso o wg.Wait() não tenha sido alcançado.
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
