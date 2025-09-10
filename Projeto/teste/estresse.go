package main

import (
	"encoding/json"
	"fmt"
	"net"
	"sync"
	"time"
)

// Variável global para quantidade de usuários
const quantidadeUsuarios = 1000 // 100 para rodar os 3 testes normais

// Estruturas do protocolo
type Mensagem struct {
	Comando string          `json:"comando"`
	Dados   json.RawMessage `json:"dados"`
}

type DadosLogin struct {
	Nome string `json:"nome"`
}

type ComprarPacoteReq struct {
	Quantidade int `json:"quantidade"`
}

type DadosPong struct {
	Timestamp int64 `json:"timestamp"`
}

type DadosPing struct {
	Timestamp int64 `json:"timestamp"`
}

// Cliente de teste
type ClienteTeste struct {
	conn    net.Conn
	nome    string
	encoder *json.Encoder
	decoder *json.Decoder
	sucesso bool
}

func (c *ClienteTeste) conectar() error {
	// Conexão direta SEM retry para máxima velocidade
	conn, err := net.Dial("tcp", "servidor:65432")
	if err != nil {
		return err
	}

	// Configurações de TCP para alta concorrência
	if tcpConn, ok := conn.(*net.TCPConn); ok {
		tcpConn.SetKeepAlive(true)
		tcpConn.SetKeepAlivePeriod(30 * time.Second)
		tcpConn.SetNoDelay(true)
	}

	c.conn = conn
	c.encoder = json.NewEncoder(conn)
	c.decoder = json.NewDecoder(conn)
	c.sucesso = true
	return nil
}

func (c *ClienteTeste) login() error {
	dados, _ := json.Marshal(DadosLogin{Nome: c.nome})
	return c.encoder.Encode(Mensagem{Comando: "LOGIN", Dados: dados})
}

func (c *ClienteTeste) entrarNaFila() error {
	return c.encoder.Encode(Mensagem{Comando: "ENTRAR_NA_FILA"})
}

func (c *ClienteTeste) comprarPacote() error {
	dados, _ := json.Marshal(ComprarPacoteReq{Quantidade: 1})
	return c.encoder.Encode(Mensagem{Comando: "COMPRAR_PACOTE", Dados: dados})
}

func (c *ClienteTeste) lerMensagens() {
	defer c.conn.Close()

	for {
		// Timeout para evitar travamento
		c.conn.SetReadDeadline(time.Now().Add(10 * time.Second))

		var msg Mensagem
		if err := c.decoder.Decode(&msg); err != nil {
			return
		}

		switch msg.Comando {
		case "PING":
			var dadosPing DadosPing
			json.Unmarshal(msg.Dados, &dadosPing)
			dados, _ := json.Marshal(DadosPong{Timestamp: dadosPing.Timestamp})
			c.encoder.Encode(Mensagem{Comando: "PONG", Dados: dados})
		case "PACOTE_RESULTADO":
			fmt.Printf("[%s] ✅ Pacote comprado com sucesso!\n", c.nome)
		case "ERRO":
			fmt.Printf("[%s] ❌ Erro recebido do servidor\n", c.nome)
		}
	}
}

// Teste 1: Estabilidade sob carga - VERSÃO ULTRA RÁPIDA
func testeEstabilidade(n int, wg *sync.WaitGroup) {
	defer wg.Done()

	fmt.Printf("🔬 TESTE 1: Estabilidade sob carga com %d clientes (ULTRA RÁPIDO)\n", n)

	var clientes []*ClienteTeste
	sucessos := 0
	var mutex sync.Mutex

	// SEM SEMÁFORO - Conexões simultâneas para máxima velocidade
	for i := 0; i < n; i++ {
		go func(id int) {
			cliente := &ClienteTeste{nome: fmt.Sprintf("EstabBot%03d", id+1)}
			if err := cliente.conectar(); err != nil {
				fmt.Printf("[%s] ❌ Erro ao conectar: %v\n", cliente.nome, err)
				return
			}

			if err := cliente.login(); err != nil {
				fmt.Printf("[%s] ❌ Erro no login: %v\n", cliente.nome, err)
				cliente.conn.Close()
				return
			}

			if err := cliente.entrarNaFila(); err != nil {
				fmt.Printf("[%s] ❌ Erro ao entrar na fila: %v\n", cliente.nome, err)
				cliente.conn.Close()
				return
			}

			mutex.Lock()
			clientes = append(clientes, cliente)
			sucessos++
			mutex.Unlock()

			fmt.Printf("[%s] ✅ Conectado e na fila\n", cliente.nome)
			go cliente.lerMensagens()
		}(i)

		// Delay muito pequeno para não sobrecarregar
		if i%10 == 0 {
			time.Sleep(1 * time.Millisecond)
		}
	}

	// Aguarda um pouco para conexões se estabelecerem
	time.Sleep(2 * time.Second)

	fmt.Printf("✅ %d/%d clientes conectados com sucesso para teste de estabilidade\n", sucessos, n)

	// Monitora por 10 segundos (reduzido de 30s)
	time.Sleep(10 * time.Second)

	// Fecha conexões
	mutex.Lock()
	for _, cliente := range clientes {
		cliente.conn.Close()
	}
	mutex.Unlock()

	fmt.Printf("🏁 Teste de estabilidade concluído! Sucessos: %d/%d\n", sucessos, n)
}

// Teste 2: Justiça na concorrência - VERSÃO ULTRA RÁPIDA
func testeJustica(n int, wg *sync.WaitGroup) {
	defer wg.Done()

	fmt.Printf("🔬 TESTE 2: Justiça na concorrência com %d clientes (ULTRA RÁPIDO)\n", n)

	var wgConcorrencia sync.WaitGroup
	sucessos := 0
	var mutex sync.Mutex
	var ready sync.WaitGroup

	// Canal para sincronizar o disparo simultâneo
	start := make(chan struct{})

	// SEM SEMÁFORO - Conexões simultâneas para máxima velocidade
	for i := 0; i < n; i++ {
		wgConcorrencia.Add(1)
		ready.Add(1)

		go func(id int) {
			defer wgConcorrencia.Done()

			cliente := &ClienteTeste{nome: fmt.Sprintf("JustBot%03d", id+1)}
			if err := cliente.conectar(); err != nil {
				fmt.Printf("[%s] ❌ Erro ao conectar: %v\n", cliente.nome, err)
				ready.Done()
				return
			}

			if err := cliente.login(); err != nil {
				fmt.Printf("[%s] ❌ Erro no login: %v\n", cliente.nome, err)
				cliente.conn.Close()
				ready.Done()
				return
			}

			// NÃO entra na fila - compra diretamente para teste de justiça
			fmt.Printf("[%s] ✅ Conectado (fora da fila)\n", cliente.nome)
			ready.Done()

			// Aguarda o sinal para disparar simultaneamente
			<-start

			// Tenta comprar pacote SIMULTANEAMENTE (fora da fila)
			if err := cliente.comprarPacote(); err != nil {
				fmt.Printf("[%s] ❌ Erro ao enviar compra: %v\n", cliente.nome, err)
				cliente.conn.Close()
				return
			}

			fmt.Printf("[%s] 📦 Tentativa de compra enviada\n", cliente.nome)

			// Aguarda um pouco para o servidor processar
			time.Sleep(200 * time.Millisecond)

			// Lê resposta com timeout menor para velocidade
			cliente.conn.SetReadDeadline(time.Now().Add(5 * time.Second))

			// Loop para ler mensagens com timeout menor
			timeout := time.After(15 * time.Second)
			sucesso := false

			for !sucesso {
				select {
				case <-timeout:
					fmt.Printf("[%s] ⏰ Timeout aguardando resposta\n", cliente.nome)
					sucesso = true
				default:
					cliente.conn.SetReadDeadline(time.Now().Add(2 * time.Second))
					var msg Mensagem
					if err := cliente.decoder.Decode(&msg); err == nil {
						if msg.Comando == "PACOTE_RESULTADO" {
							fmt.Printf("[%s] ✅ Pacote obtido com sucesso!\n", cliente.nome)
							mutex.Lock()
							sucessos++
							mutex.Unlock()
							sucesso = true
						} else if msg.Comando == "ERRO" {
							fmt.Printf("[%s] ❌ Erro ao obter pacote\n", cliente.nome)
							sucesso = true
						} else if msg.Comando == "PING" {
							// Responde ao ping
							var dadosPing DadosPing
							json.Unmarshal(msg.Dados, &dadosPing)
							dados, _ := json.Marshal(DadosPong{Timestamp: dadosPing.Timestamp})
							cliente.encoder.Encode(Mensagem{Comando: "PONG", Dados: dados})
						}
					} else {
						// Timeout na leitura, tenta novamente
						time.Sleep(100 * time.Millisecond)
					}
				}
			}

			// Fecha conexão rapidamente
			cliente.conn.Close()
		}(i)

		// Delay muito pequeno para não sobrecarregar
		if i%10 == 0 {
			time.Sleep(1 * time.Millisecond)
		}
	}

	// Aguarda um pouco para conexões se estabelecerem
	time.Sleep(3 * time.Second)

	// Aguarda todos estarem prontos
	ready.Wait()
	fmt.Printf("🚀 Todos os clientes prontos! Disparando compras simultâneas...\n")

	// Pequena pausa para garantir que todos estão sincronizados
	time.Sleep(500 * time.Millisecond)

	// Dispara todos ao mesmo tempo
	close(start)

	wgConcorrencia.Wait()
	fmt.Printf("🏁 Teste de justiça concluído! Sucessos: %d/%d\n", sucessos, n)
}

// Teste 3: Múltiplas conexões simultâneas - VERSÃO ULTRA RÁPIDA
func testeConexoes(n int, wg *sync.WaitGroup) {
	defer wg.Done()

	fmt.Printf("🔬 TESTE 3: Múltiplas conexões simultâneas com %d clientes (ULTRA RÁPIDO)\n", n)

	var clientes []*ClienteTeste
	sucessos := 0
	var mutex sync.Mutex
	inicio := time.Now()

	// SEM SEMÁFORO - Conexões simultâneas para máxima velocidade
	for i := 0; i < n; i++ {
		go func(id int) {
			cliente := &ClienteTeste{nome: fmt.Sprintf("ConexBot%03d", id+1)}
			if err := cliente.conectar(); err != nil {
				fmt.Printf("[%s] ❌ Erro ao conectar: %v\n", cliente.nome, err)
				return
			}

			if err := cliente.login(); err != nil {
				fmt.Printf("[%s] ❌ Erro no login: %v\n", cliente.nome, err)
				cliente.conn.Close()
				return
			}

			if err := cliente.entrarNaFila(); err != nil {
				fmt.Printf("[%s] ❌ Erro ao entrar na fila: %v\n", cliente.nome, err)
				cliente.conn.Close()
				return
			}

			mutex.Lock()
			clientes = append(clientes, cliente)
			sucessos++
			mutex.Unlock()

			fmt.Printf("[%s] ✅ Conectado e na fila\n", cliente.nome)
			go cliente.lerMensagens()
		}(i)

		// Delay muito pequeno para não sobrecarregar
		if i%10 == 0 {
			time.Sleep(1 * time.Millisecond)
		}
	}

	// Aguarda um pouco para conexões se estabelecerem
	time.Sleep(2 * time.Second)

	duracao := time.Since(inicio)
	fmt.Printf("✅ %d/%d clientes conectados com sucesso em %v\n", sucessos, n, duracao)

	// Monitora por 5 segundos (reduzido de 20s)
	time.Sleep(5 * time.Second)

	// Fecha conexões
	mutex.Lock()
	for _, cliente := range clientes {
		cliente.conn.Close()
	}
	mutex.Unlock()

	fmt.Printf("🏁 Teste de conexões concluído! Sucessos: %d/%d\n", sucessos, n)
}

// Executa todos os testes com n goroutines cada
func executarTodosTestes(n int) {
	fmt.Println("🎯 SISTEMA DE TESTES DE ESTRESSE")
	fmt.Println("=================================")
	fmt.Printf("Executando %d goroutines para cada teste\n", n)

	// Aguarda servidor estar pronto
	fmt.Println("⏳ Aguardando servidor...")
	for i := 0; i < 60; i++ { // Aumentado para 60 segundos
		conn, err := net.Dial("tcp", "servidor:65432")
		if err == nil {
			conn.Close()
			fmt.Println("✅ Servidor pronto!")
			break
		}
		time.Sleep(1 * time.Second)
	}

	time.Sleep(3 * time.Second) // Aguarda estabilização

	var wg sync.WaitGroup

	// Executa os 3 testes em paralelo
	wg.Add(3)
	go testeEstabilidade(n, &wg)
	go testeJustica(n, &wg)
	go testeConexoes(n, &wg)

	wg.Wait()

	fmt.Println("\n✅ TODOS OS TESTES CONCLUÍDOS!")
	fmt.Println("📋 Verifique os logs do servidor para análise dos resultados")
}

// Teste específico para alta concorrência (10.000 clientes)
func testeAltaConcorrencia() {
	fmt.Println("🚀 TESTE ULTRA RÁPIDO - 10.000 CLIENTES (SEM GARGALOS)")
	fmt.Println("=======================================================")

	// Aguarda servidor estar pronto
	fmt.Println("⏳ Aguardando servidor...")
	for i := 0; i < 30; i++ {
		conn, err := net.Dial("tcp", "servidor:65432")
		if err == nil {
			conn.Close()
			fmt.Println("✅ Servidor pronto!")
			break
		}
		time.Sleep(1 * time.Second)
	}

	time.Sleep(1 * time.Second) // Mínimo necessário

	// Testa com 10.000 clientes
	n := 10000
	fmt.Printf("🔥 Conectando %d clientes ULTRA RÁPIDO (SEM DELAYS)...\n", n)

	var wg sync.WaitGroup
	sucessos := 0
	var mutex sync.Mutex
	inicio := time.Now()

	// SEM SEMÁFORO - TODAS as conexões simultâneas para máxima velocidade

	for i := 0; i < n; i++ {
		wg.Add(1)

		go func(id int) {
			defer wg.Done()

			cliente := &ClienteTeste{nome: fmt.Sprintf("StressBot%05d", id+1)}
			if err := cliente.conectar(); err != nil {
				return
			}

			if err := cliente.login(); err != nil {
				cliente.conn.Close()
				return
			}

			if err := cliente.entrarNaFila(); err != nil {
				cliente.conn.Close()
				return
			}

			mutex.Lock()
			sucessos++
			mutex.Unlock()

			// Mantém conexão por apenas 2 segundos (reduzido de 30s)
			time.Sleep(2 * time.Second)
			cliente.conn.Close()
		}(i)
		// SEM DELAYS - conexões totalmente simultâneas
	}

	wg.Wait()
	duracao := time.Since(inicio)

	fmt.Printf("🏁 Teste ultra rápido concluído!\n")
	fmt.Printf("📊 Resultados:\n")
	fmt.Printf("   - Clientes conectados: %d/%d\n", sucessos, n)
	fmt.Printf("   - Tempo total: %v\n", duracao)
	fmt.Printf("   - Taxa de conexão: %.2f clientes/segundo\n", float64(sucessos)/duracao.Seconds())
	fmt.Printf("   - Taxa de sucesso: %.2f%%\n", float64(sucessos)/float64(n)*100)
}

func main() {
	// Escolhe qual teste executar baseado na quantidade
	if quantidadeUsuarios <= 100 {
		fmt.Printf("🚀 Executando 3 TESTES ULTRA RÁPIDOS com %d clientes cada\n", quantidadeUsuarios)
		executarTodosTestes(quantidadeUsuarios)
	} else {
		testeAltaConcorrencia()
	}
}
