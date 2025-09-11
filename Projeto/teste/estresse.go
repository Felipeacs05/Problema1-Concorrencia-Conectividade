// felipeacs05/problema1-concorrencia-conectividade/Problema1-Concorrencia-Conectividade-77d73bcc575bbc2b6e076d0c153ffc2b7b175855/Projeto/teste/estresse.go
package main

import (
	"encoding/json"
	"fmt"
	"net"
	"sync"
	"time"
)

// Variável global para quantidade de usuários
const quantidadeUsuarios = 20000

// OTIMIZAÇÃO: Limita o número de conexões simultâneas que o CLIENTE tenta fazer.
// Isso evita que o próprio cliente se torne o gargalo.
const maxConcurrentConnects = 300

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
type ClienteTeste struct {
	conn    net.Conn
	nome    string
	encoder *json.Encoder
	decoder *json.Decoder
}

func (c *ClienteTeste) conectar() error {
	conn, err := net.DialTimeout("tcp", "servidor:65432", 10*time.Second)
	if err != nil {
		return err
	}
	if tcpConn, ok := conn.(*net.TCPConn); ok {
		tcpConn.SetNoDelay(true)
	}
	c.conn = conn
	c.encoder = json.NewEncoder(conn)
	c.decoder = json.NewDecoder(conn)
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
		c.conn.SetReadDeadline(time.Now().Add(20 * time.Second))
		var msg Mensagem
		if err := c.decoder.Decode(&msg); err != nil {
			return
		}
	}
}

// Teste 1: Estabilidade sob carga
func testeEstabilidade(n int, wg *sync.WaitGroup, resultChan chan<- string) {
	defer wg.Done()
	sucessos := 0
	var mutex sync.Mutex
	var wgConexoes sync.WaitGroup
	connectSemaphore := make(chan struct{}, maxConcurrentConnects)

	for i := 0; i < n; i++ {
		wgConexoes.Add(1)
		connectSemaphore <- struct{}{} // Adquire uma vaga para conectar
		go func(id int) {
			defer func() {
				<-connectSemaphore // Libera a vaga
				wgConexoes.Done()
			}()

			cliente := &ClienteTeste{nome: fmt.Sprintf("EstabBot%04d", id+1)}
			if err := cliente.conectar(); err != nil {
				return
			}
			defer cliente.conn.Close()
			if err := cliente.login(); err != nil {
				return
			}
			if err := cliente.entrarNaFila(); err != nil {
				return
			}
			mutex.Lock()
			sucessos++
			mutex.Unlock()
			go cliente.lerMensagens()
		}(i)
	}
	wgConexoes.Wait()
	time.Sleep(10 * time.Second)
	resultChan <- fmt.Sprintf("🏁 Teste de estabilidade concluído! Sucessos: %d/%d", sucessos, n)
}

// Teste 2: Justiça na concorrência
func testeJustica(n int, wg *sync.WaitGroup, resultChan chan<- string) {
	defer wg.Done()
	var wgConcorrencia sync.WaitGroup
	var wgProntos sync.WaitGroup
	sucessos := 0
	var mutex sync.Mutex
	start := make(chan struct{})
	connectSemaphore := make(chan struct{}, maxConcurrentConnects)

	for i := 0; i < n; i++ {
		wgConcorrencia.Add(1)
		wgProntos.Add(1)
		connectSemaphore <- struct{}{} // Adquire uma vaga para conectar
		go func(id int) {
			defer wgConcorrencia.Done()
			<-connectSemaphore // Libera a vaga assim que a conexão é feita (ou falha)

			cliente := &ClienteTeste{nome: fmt.Sprintf("JustBot%04d", id+1)}
			if err := cliente.conectar(); err != nil {
				wgProntos.Done()
				return
			}
			defer cliente.conn.Close()
			if err := cliente.login(); err != nil {
				wgProntos.Done()
				return
			}

			wgProntos.Done()
			<-start
			if err := cliente.comprarPacote(); err != nil {
				return
			}

			timeout := time.After(20 * time.Second)
			for {
				select {
				case <-timeout:
					return
				default:
					cliente.conn.SetReadDeadline(time.Now().Add(5 * time.Second))
					var msg Mensagem
					if err := cliente.decoder.Decode(&msg); err != nil {
						return
					}
					if msg.Comando == "PACOTE_RESULTADO" {
						mutex.Lock()
						sucessos++
						mutex.Unlock()
						return
					}
				}
			}
		}(i)
	}
	wgProntos.Wait()
	close(start)
	wgConcorrencia.Wait()
	resultChan <- fmt.Sprintf("🏁 Teste de justiça concluído! Sucessos: %d/%d", sucessos, n)
}

// Teste 3: Múltiplas conexões simultâneas
func testeConexoes(n int, wg *sync.WaitGroup, resultChan chan<- string) {
	defer wg.Done()
	sucessos := 0
	var mutex sync.Mutex
	inicio := time.Now()
	var wgConexoes sync.WaitGroup
	connectSemaphore := make(chan struct{}, maxConcurrentConnects)

	for i := 0; i < n; i++ {
		wgConexoes.Add(1)
		connectSemaphore <- struct{}{} // Adquire uma vaga para conectar
		go func(id int) {
			defer func() {
				<-connectSemaphore // Libera a vaga
				wgConexoes.Done()
			}()

			cliente := &ClienteTeste{nome: fmt.Sprintf("ConexBot%04d", id+1)}
			if err := cliente.conectar(); err != nil {
				return
			}
			defer cliente.conn.Close()
			if err := cliente.login(); err != nil {
				return
			}
			mutex.Lock()
			sucessos++
			mutex.Unlock()
		}(i)
	}
	wgConexoes.Wait()
	duracao := time.Since(inicio)
	taxa := float64(sucessos) / duracao.Seconds()
	resultChan <- fmt.Sprintf("🏁 Teste de conexões concluído! Sucessos: %d/%d ;Taxa: %.2f conexões/segundo", sucessos, n, taxa)
}

func executarTodosTestes(n int) {
	fmt.Println("🎯 SISTEMA DE TESTES DE ESTRESSE")
	fmt.Println("=================================")
	estabilidadeResultChan := make(chan string, 1)
	justicaResultChan := make(chan string, 1)
	conexoesResultChan := make(chan string, 1)
	var wg sync.WaitGroup
	wg.Add(3)

	fmt.Printf("🔬 TESTE 1: Estabilidade sob carga com %d clientes\n", n)
	fmt.Printf("🔬 TESTE 2: Justiça na concorrência com %d clientes\n", n)
	fmt.Printf("🔬 TESTE 3: Múltiplas conexões simultâneas com %d clientes\n", n)

	go testeEstabilidade(n, &wg, estabilidadeResultChan)
	go testeJustica(n, &wg, justicaResultChan)
	go testeConexoes(n, &wg, conexoesResultChan)
	wg.Wait()

	conexoesResult := <-conexoesResultChan
	justicaResult := <-justicaResultChan
	estabilidadeResult := <-estabilidadeResultChan

	fmt.Println("\n-- Final do teste de estresse --")
	fmt.Println(conexoesResult)
	fmt.Println(justicaResult)
	fmt.Println(estabilidadeResult)
}

func main() {
	executarTodosTestes(quantidadeUsuarios)
}
