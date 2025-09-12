package main

import (
	"encoding/json"
	"fmt"
	"log"
	"meujogo/protocolo"
	"net"
	"sync"
	"time"
)

// Bot representa um cliente de teste automatizado.
type Bot struct {
	ID         int
	Conn       net.Conn
	Encoder    *json.Encoder
	Decoder    *json.Decoder
	Nome       string
	Inventario []protocolo.Carta
	done       chan bool
	pingStart  time.Time
	mu         sync.Mutex
}

// NovoBot cria e conecta um novo bot ao servidor.
func NovoBot(id int, serverAddr string) (*Bot, error) {
	conn, err := net.DialTimeout("tcp", serverAddr, 10*time.Second)
	if err != nil {
		return nil, fmt.Errorf("bot %d: falha ao conectar: %v", id, err)
	}

	bot := &Bot{
		ID:      id,
		Conn:    conn,
		Encoder: json.NewEncoder(conn),
		Decoder: json.NewDecoder(conn),
		Nome:    fmt.Sprintf("Bot-%d", id),
		done:    make(chan bool),
	}
	return bot, nil
}

// Executar inicia a lógica do bot.
func (b *Bot) Executar() {
	defer b.Conn.Close()
	go b.lerServidor()

	// 1. Login
	b.enviarComando("LOGIN", protocolo.DadosLogin{Nome: b.Nome})

	// 2. Entrar na fila
	b.enviarComando("ENTRAR_NA_FILA", nil)

	// Aguarda o fim do jogo
	<-b.done
	log.Printf("Bot %d (%s) terminou.", b.ID, b.Nome)
}

func (b *Bot) enviarComando(comando string, dados interface{}) {
	var rawDados json.RawMessage
	if dados != nil {
		jsonBytes, err := json.Marshal(dados)
		if err != nil {
			log.Printf("Bot %d: erro ao serializar dados: %v", b.ID, err)
			return
		}
		rawDados = jsonBytes
	}

	msg := protocolo.Mensagem{Comando: comando, Dados: rawDados}
	if err := b.Encoder.Encode(msg); err != nil {
		log.Printf("Bot %d: erro ao enviar comando '%s': %v", b.ID, comando, err)
	}
}

func (b *Bot) lerServidor() {
	for {
		var msg protocolo.Mensagem
		if err := b.Decoder.Decode(&msg); err != nil {
			log.Printf("❌ Bot %d (%s) desconectado inesperadamente.", b.ID, b.Nome)
			close(b.done)
			return
		}

		switch msg.Comando {
		case "PARTIDA_ENCONTRADA":
			// 3. Comprar pacote para iniciar
			b.enviarComando("COMPRAR_PACOTE", protocolo.ComprarPacoteReq{Quantidade: 1})

		case "PACOTE_RESULTADO":
			var resp protocolo.ComprarPacoteResp
			if json.Unmarshal(msg.Dados, &resp) == nil {
				b.Inventario = resp.Cartas
			}

		case "ATUALIZACAO_JOGO":
			// 4. Jogar a primeira carta válida
			if len(b.Inventario) > 0 {
				cartaAJogar := b.Inventario[0]
				b.Inventario = b.Inventario[1:] // Remove a carta da mão (simulação simples)
				b.enviarComando("JOGAR_CARTA", protocolo.DadosJogarCarta{CartaID: cartaAJogar.ID})
			}

		case "FIM_DE_JOGO":
			// 5. O jogo acabou, medir a latência final e depois sair.
			b.mu.Lock()
			b.pingStart = time.Now()
			b.mu.Unlock()
			b.enviarComando("PING", protocolo.DadosPing{Timestamp: time.Now().UnixMilli()})
			// Não fecha o 'done' aqui, aguarda o PONG para calcular a latência.
			return

		case "PONG":
			var dadosPong protocolo.DadosPong
			if json.Unmarshal(msg.Dados, &dadosPong) == nil {
				b.mu.Lock()
				start := b.pingStart
				b.mu.Unlock()
				// Verifica se estávamos esperando por este PONG para finalizar.
				if !start.IsZero() {
					latencia := time.Since(start)
					// Usa float64 para obter precisão decimal em milissegundos.
					log.Printf("✅ Bot %d (%s) terminou. Latência final: %.3fms", b.ID, b.Nome, float64(latencia.Microseconds())/1000.0)
					close(b.done)
					return
				}
			}

		case "PING":
			var dadosPing protocolo.DadosPing
			if json.Unmarshal(msg.Dados, &dadosPing) == nil {
				b.enviarComando("PONG", protocolo.DadosPong{Timestamp: dadosPing.Timestamp})
			}
		}
	}
}

func main() {
	// ==================================================
	// CONFIGURE AQUI O NÚMERO DE BOTS PARA O TESTE
	const numBots = 100
	// ==================================================

	serverAddr := "servidor:65432"
	// Opcional: Para testar localmente fora do Docker, você pode mudar para:
	// serverAddr = "localhost:65432"

	log.Printf("Iniciando teste de estresse com %d bots contra %s...", numBots, serverAddr)

	var wg sync.WaitGroup
	wg.Add(numBots)

	for i := 0; i < numBots; i++ {
		time.Sleep(10 * time.Millisecond) // Pequeno intervalo para não sobrecarregar o accept() do servidor de uma vez
		go func(botID int) {
			defer wg.Done()
			bot, err := NovoBot(botID, serverAddr)
			if err != nil {
				log.Printf("❌ Erro ao criar bot %d: %v", botID, err)
				return
			}
			bot.Executar()
		}(i)
	}

	log.Println("Todos os bots foram iniciados. Aguardando a conclusão...")
	wg.Wait()
	log.Println("Teste de estresse concluído.")
}
