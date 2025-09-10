package main

import (
	"encoding/json"
	"fmt"
	"net"
	"sync"
	"time"
)

// Variável global para quantidade de usuários
const quantidadeUsuarios = 20000 // 100 para rodar os 3 testes normais

// Estruturas do protocolo (sem alterações)
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
	sucesso bool // Este campo não estava sendo usado, pode ser removido se não tiver outro propósito.
}

func (c *ClienteTeste) conectar() error {
	conn, err := net.Dial("tcp", "servidor:65432")
	if err != nil {
		return err
	}

	if tcpConn, ok := conn.(*net.TCPConn); ok {
		tcpConn.SetKeepAlive(true)
		tcpConn.SetKeepAlivePeriod(30 * time.Second)
		tcpConn.SetNoDelay(true)
	}

	c.conn = conn
	c.encoder = json.NewEncoder(conn)
	c.decoder = json.NewDecoder(conn)
	return nil
}

func (c *ClienteTeste) login() error {
	// MELHORIA: Tratar o erro potencial do json.Marshal.
	dadosLogin := DadosLogin{Nome: c.nome}
	dados, err := json.Marshal(dadosLogin)
	if err != nil {
		return fmt.Errorf("falha ao serializar dados de login: %w", err)
	}
	return c.encoder.Encode(Mensagem{Comando: "LOGIN", Dados: dados})
}

func (c *ClienteTeste) entrarNaFila() error {
	return c.encoder.Encode(Mensagem{Comando: "ENTRAR_NA_FILA"})
}

func (c *ClienteTeste) comprarPacote() error {
	// MELHORIA: Tratar o erro potencial do json.Marshal.
	req := ComprarPacoteReq{Quantidade: 1}
	dados, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("falha ao serializar dados de compra: %w", err)
	}
	return c.encoder.Encode(Mensagem{Comando: "COMPRAR_PACOTE", Dados: dados})
}

func (c *ClienteTeste) lerMensagens() {
	// Garante que a conexão seja fechada quando esta função terminar (seja por erro ou fim do teste).
	defer c.conn.Close()

	for {
		// Define um deadline para cada tentativa de leitura.
		c.conn.SetReadDeadline(time.Now().Add(10 * time.Second))

		var msg Mensagem
		err := c.decoder.Decode(&msg)

		// CORREÇÃO: Tratamento de erro robusto para não matar a goroutine silenciosamente.
		if err != nil {
			// Se o erro for um timeout, é algo esperado. Apenas continuamos para a próxima iteração.
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue
			}
			// Se o erro for io.EOF, significa que o servidor fechou a conexão de forma limpa.
			// Qualquer outro erro também é considerado fatal para esta conexão.
			// A função então retorna, e o 'defer c.conn.Close()' é executado.
			// fmt.Printf("[%s] 🔌 Conexão encerrada: %v\n", c.nome, err) // Descomente para depuração
			return
		}

		switch msg.Comando {
		case "PING":
			var dadosPing DadosPing
			json.Unmarshal(msg.Dados, &dadosPing) // Erro de unmarshal aqui é menos crítico para um cliente de teste.
			dadosPong := DadosPong{Timestamp: dadosPing.Timestamp}
			dados, err := json.Marshal(dadosPong)
			if err == nil {
				c.encoder.Encode(Mensagem{Comando: "PONG", Dados: dados})
			}
		case "PACOTE_RESULTADO":
			fmt.Printf("[%s] ✅ Pacote comprado com sucesso!\n", c.nome)
		case "ERRO":
			fmt.Printf("[%s] ❌ Erro recebido do servidor\n", c.nome)
		}
	}
}

// Teste 1: Estabilidade sob carga
func testeEstabilidade(n int, wg *sync.WaitGroup) {
	defer wg.Done()
	fmt.Printf("🔬 TESTE 1: Estabilidade sob carga com %d clientes\n", n)

	var clientes []*ClienteTeste
	sucessos := 0
	var mutex sync.Mutex

	// CORREÇÃO: Usar um WaitGroup para sincronização confiável em vez de time.Sleep.
	var wgConexoes sync.WaitGroup

	for i := 0; i < n; i++ {
		wgConexoes.Add(1)
		go func(id int) {
			defer wgConexoes.Done()

			cliente := &ClienteTeste{nome: fmt.Sprintf("EstabBot%03d", id+1)}
			if err := cliente.conectar(); err != nil {
				return
			}
			// MELHORIA: Garante que a conexão seja fechada se algo der errado no login ou na fila.
			// A goroutine lerMensagens tem seu próprio defer, então isso só afeta as falhas antes dela iniciar.
			defer func() {
				if r := recover(); r != nil {
					cliente.conn.Close()
				}
			}()

			if err := cliente.login(); err != nil {
				cliente.conn.Close()
				return
			}
			if err := cliente.entrarNaFila(); err != nil {
				cliente.conn.Close()
				return
			}

			mutex.Lock()
			clientes = append(clientes, cliente)
			sucessos++
			mutex.Unlock()

			fmt.Printf("[%s] ✅ Conectado e na fila\n", cliente.nome)
			// A função lerMensagens agora é responsável pelo ciclo de vida da conexão.
			go cliente.lerMensagens()
		}(i)
	}

	// CORREÇÃO: Espera TODAS as goroutines de conexão terminarem antes de continuar.
	wgConexoes.Wait()
	fmt.Printf("✅ %d/%d clientes conectados com sucesso para teste de estabilidade\n", sucessos, n)

	// Monitora por 10 segundos
	time.Sleep(10 * time.Second)

	// Ao final do teste, fechamos todas as conexões que ainda possam estar ativas.
	// O `lerMensagens` já fecha a conexão ao sair, mas isso garante o encerramento.
	mutex.Lock()
	for _, cliente := range clientes {
		cliente.conn.Close() // Fechar uma conexão já fechada é seguro em Go.
	}
	mutex.Unlock()

	fmt.Printf("🏁 Teste de estabilidade concluído! Sucessos: %d/%d\n", sucessos, n)
}

// Teste 2: Justiça na concorrência
func testeJustica(n int, wg *sync.WaitGroup) {
	defer wg.Done()
	fmt.Printf("🔬 TESTE 2: Justiça na concorrência com %d clientes\n", n)

	var wgConcorrencia sync.WaitGroup
	sucessos := 0
	var mutex sync.Mutex

	start := make(chan struct{})

	for i := 0; i < n; i++ {
		wgConcorrencia.Add(1)
		go func(id int) {
			defer wgConcorrencia.Done()

			cliente := &ClienteTeste{nome: fmt.Sprintf("JustBot%03d", id+1)}
			if err := cliente.conectar(); err != nil {
				return
			}
			defer cliente.conn.Close() // Garante que a conexão feche ao final da goroutine.

			if err := cliente.login(); err != nil {
				return
			}

			fmt.Printf("[%s] ✅ Conectado (pronto para comprar)\n", cliente.nome)
			<-start // Aguarda o sinal para disparar

			if err := cliente.comprarPacote(); err != nil {
				return
			}
			fmt.Printf("[%s] 📦 Tentativa de compra enviada\n", cliente.nome)

			// Loop de leitura com timeout
			timeout := time.After(15 * time.Second)
			for {
				select {
				case <-timeout:
					fmt.Printf("[%s] ⏰ Timeout aguardando resposta de compra\n", cliente.nome)
					return
				default:
					cliente.conn.SetReadDeadline(time.Now().Add(2 * time.Second))
					var msg Mensagem
					err := cliente.decoder.Decode(&msg)

					// CORREÇÃO: Lógica de tratamento de erro robusta, igual à de lerMensagens.
					if err != nil {
						if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
							continue // Timeout na leitura é normal, tenta de novo.
						}
						// Erro fatal na conexão, sai do loop de leitura.
						return
					}

					if msg.Comando == "PACOTE_RESULTADO" {
						fmt.Printf("[%s] ✅ Pacote obtido com sucesso!\n", cliente.nome)
						mutex.Lock()
						sucessos++
						mutex.Unlock()
						return // Sucesso, encerra a goroutine.
					} else if msg.Comando == "ERRO" {
						fmt.Printf("[%s] ❌ Erro ao obter pacote\n", cliente.nome)
						return // Erro, encerra a goroutine.
					}
					// Ignora outras mensagens como PING aqui para focar no resultado.
				}
			}
		}(i)
	}

	// Aguarda um tempo razoável para os clientes se conectarem e ficarem prontos.
	// Em um teste real, o ideal seria usar outro WaitGroup aqui (como o 'ready' do seu código original).
	time.Sleep(3 * time.Second)
	fmt.Printf("🚀 Todos os clientes prontos! Disparando compras simultâneas...\n")

	close(start) // Dispara todos ao mesmo tempo
	wgConcorrencia.Wait()
	fmt.Printf("🏁 Teste de justiça concluído! Sucessos: %d/%d\n", sucessos, n)
}

// Teste 3: Múltiplas conexões simultâneas
func testeConexoes(n int, wg *sync.WaitGroup) {
	defer wg.Done()
	fmt.Printf("🔬 TESTE 3: Múltiplas conexões simultâneas com %d clientes\n", n)

	sucessos := 0
	var mutex sync.Mutex
	inicio := time.Now()

	// CORREÇÃO: Usar um WaitGroup para sincronização confiável.
	var wgConexoes sync.WaitGroup

	for i := 0; i < n; i++ {
		wgConexoes.Add(1)
		go func(id int) {
			defer wgConexoes.Done()

			cliente := &ClienteTeste{nome: fmt.Sprintf("ConexBot%03d", id+1)}
			if err := cliente.conectar(); err != nil {
				return
			}
			defer cliente.conn.Close()

			if err := cliente.login(); err != nil {
				return
			}

			// Apenas conecta e faz login para testar a capacidade de aceitar conexões.
			mutex.Lock()
			sucessos++
			mutex.Unlock()
		}(i)
	}

	// CORREÇÃO: Espera todas as conexões serem estabelecidas.
	wgConexoes.Wait()
	duracao := time.Since(inicio)

	fmt.Printf("✅ %d/%d clientes conectados com sucesso em %v\n", sucessos, n, duracao)
	fmt.Printf("🏁 Teste de conexões concluído! Taxa: %.2f conexões/segundo\n", float64(sucessos)/duracao.Seconds())
}

func executarTodosTestes(n int) {
	fmt.Println("🎯 SISTEMA DE TESTES DE ESTRESSE")
	fmt.Println("=================================")
	fmt.Printf("Executando %d goroutines para cada teste\n", n)

	// Aguarda servidor estar pronto...
	time.Sleep(3 * time.Second)

	var wg sync.WaitGroup
	wg.Add(3)
	go testeEstabilidade(n, &wg)
	go testeJustica(n, &wg)
	go testeConexoes(n, &wg)
	wg.Wait()

	fmt.Println("\n✅ TODOS OS TESTES CONCLUÍDOS!")
}

func main() {
	// ---------------------------------------------------------------------------------
	// ATENÇÃO: AVISO IMPORTANTE SOBRE LIMITES DO SISTEMA OPERACIONAL
	// ---------------------------------------------------------------------------------
	// Para rodar testes com muitos usuários (acima de ~1000), você PRECISA
	// aumentar o limite de "descritores de arquivos abertos" do seu sistema.
	// No Linux ou macOS, execute este comando no terminal ANTES de rodar o teste:
	//
	// ulimit -n 20000
	//
	// Senão, o teste irá falhar com o erro "too many open files".
	// ---------------------------------------------------------------------------------

	fmt.Printf("🚀 Executando 3 TESTES com %d clientes cada\n", quantidadeUsuarios)
	executarTodosTestes(quantidadeUsuarios)
}
