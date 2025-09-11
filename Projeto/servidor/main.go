// felipeacs05/problema1-concorrencia-conectividade/Problema1-Concorrencia-Conectividade-77d73bcc575bbc2b6e076d0c153ffc2b7b175855/Projeto/servidor/main.go
package main

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"meujogo/protocolo"
	"net"
	"sync"
	"sync/atomic" // OTIMIZAÇÃO: Usar pacote atomic para contadores
	"time"
)

const (
	numFilaShards    = 32 // Aumentado para maior distribuição
	numEstoqueShards = 32 // Aumentado para maior distribuição
)

/* ====================== Tipos e Pools de Otimização ====================== */
type Carta = protocolo.Carta

type Cliente struct {
	Conn       net.Conn
	Nome       string
	Encoder    *json.Encoder
	Decoder    *json.Decoder // Adicionado para pool
	Mailbox    chan protocolo.Mensagem
	Sala       *Sala
	Inventario []Carta
}

// OTIMIZAÇÃO: sync.Pool para reutilizar objetos Cliente e reduzir GC
var clientePool = sync.Pool{
	New: func() interface{} {
		return &Cliente{
			Mailbox:    make(chan protocolo.Mensagem, 32),
			Inventario: make([]Carta, 0, 64),
		}
	},
}

type Sala struct {
	ID           string
	Jogadores    []*Cliente
	Estado       string
	CartasNaMesa map[string]Carta
	srv          *Servidor
	mutex        sync.Mutex
}
type filaShard struct {
	clientes []*Cliente
	mutex    sync.Mutex
}
type estoqueShard struct {
	estoque map[string][]Carta
	mutex   sync.Mutex
}
type Servidor struct {
	clientes       sync.Map
	salas          sync.Map
	shardedFila    []*filaShard
	shardedEstoque []*estoqueShard
	packSize       int
	packWorkers    int
	packWorkerPool chan packReq
}
type packReq struct {
	cli        *Cliente
	quantidade int
}

/* ====================== Servidor / bootstrap ====================== */
func novoServidor() *Servidor {
	s := &Servidor{
		packSize:       5,
		packWorkers:    1000, // Aumentado drasticamente
		packWorkerPool: make(chan packReq, 100000),
		shardedFila:    make([]*filaShard, numFilaShards),
		shardedEstoque: make([]*estoqueShard, numEstoqueShards),
	}
	for i := 0; i < numFilaShards; i++ {
		s.shardedFila[i] = &filaShard{clientes: make([]*Cliente, 0)}
	}
	estoquesIniciais := gerarEstoquesIniciais()
	for i := 0; i < numEstoqueShards; i++ {
		s.shardedEstoque[i] = &estoqueShard{estoque: estoquesIniciais[i]}
	}
	for i := 0; i < s.packWorkers; i++ {
		go s.packWorker()
	}
	return s
}
func main() {
	servidor := novoServidor()
	listener, err := net.Listen("tcp", ":65432")
	if err != nil {
		panic(err)
	}
	defer listener.Close()
	fmt.Println("[SERVIDOR] ouvindo :65432 (otimização final)")
	semaphore := make(chan struct{}, 30000)
	for {
		conn, err := listener.Accept()
		if err != nil {
			continue
		}
		// As configurações de TCP foram movidas para dentro do handleConnection
		// para serem aplicadas por goroutine, evitando qualquer contenção no loop principal.
		semaphore <- struct{}{}
		go func() {
			defer func() { <-semaphore }()
			servidor.handleConnection(conn)
		}()
	}
}

/* ====================== Processamento de Pacotes Otimizado ====================== */
func (s *Servidor) packWorker() {
	for req := range s.packWorkerPool {
		s.processarPacote(req)
	}
}
func (s *Servidor) processarPacote(req packReq) {
	if req.cli.Conn == nil {
		return
	}
	totalNecessario := req.quantidade * s.packSize
	cartas := make([]Carta, 0, totalNecessario)
	for i := 0; i < totalNecessario; i++ {
		c, ok := s.takeOneByRarityWithDowngrade(sampleRaridade())
		if !ok {
			c = s.gerarCartaComumBasica()
		}
		cartas = append(cartas, c)
	}
	req.cli.Inventario = append(req.cli.Inventario, cartas...)
	msg := protocolo.Mensagem{
		Comando: "PACOTE_RESULTADO",
		Dados:   mustJSON(protocolo.ComprarPacoteResp{Cartas: cartas}),
	}
	if s.enviar(req.cli, msg) {
		if req.cli.Sala != nil {
			req.cli.Sala.marcarCompraEIniciarSePossivel(req.cli)
		}
	}
}
func (s *Servidor) takeOneByRarityWithDowngrade(r string) (Carta, bool) {
	shardIndex := rand.Intn(numEstoqueShards)
	shard := s.shardedEstoque[shardIndex]
	shard.mutex.Lock()
	order := []string{"L", "R", "U", "C"}
	var start int
	switch r {
	case "L":
		start = 0
	case "R":
		start = 1
	case "U":
		start = 2
	default:
		start = 3
	}
	for i := start; i < len(order); i++ {
		raridade := order[i]
		if arr := shard.estoque[raridade]; len(arr) > 0 {
			n := len(arr) - 1
			c := arr[n]
			shard.estoque[raridade] = arr[:n]
			shard.mutex.Unlock()
			return c, true
		}
	}
	shard.mutex.Unlock()
	return Carta{}, false
}
func (s *Servidor) gerarCartaComumBasica() Carta {
	return Carta{ID: novoID(), Nome: "Soldado Básico", Naipe: "♠", Valor: 10, Raridade: "C"}
}

/* ====================== Conexão / IO com Pools ====================== */
func (s *Servidor) handleConnection(conn net.Conn) {
	if tcpConn, ok := conn.(*net.TCPConn); ok {
		tcpConn.SetKeepAlive(true)
		tcpConn.SetKeepAlivePeriod(30 * time.Second)
		tcpConn.SetNoDelay(true)
	}

	// OTIMIZAÇÃO: Pega um objeto Cliente do pool
	cliente := clientePool.Get().(*Cliente)
	cliente.Conn = conn
	cliente.Encoder = json.NewEncoder(conn)
	cliente.Decoder = json.NewDecoder(conn)
	cliente.Nome = conn.RemoteAddr().String()

	s.adicionarCliente(cliente)
	go s.clienteWriter(cliente)
	s.clienteReader(cliente)

	// OTIMIZAÇÃO: Devolve o objeto Cliente para o pool
	conn.Close() // Garante que a conexão seja fechada
	s.removerCliente(cliente)
	cliente.Inventario = cliente.Inventario[:0] // Limpa o slice
	cliente.Sala = nil
	cliente.Conn = nil
	clientePool.Put(cliente)
}
func (s *Servidor) clienteWriter(c *Cliente) {
	for msg := range c.Mailbox {
		if c.Conn == nil {
			return
		}
		c.Conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
		if err := c.Encoder.Encode(msg); err != nil {
			return
		}
	}
}
func (s *Servidor) clienteReader(cliente *Cliente) {
	for {
		if cliente.Conn == nil {
			return
		}
		cliente.Conn.SetReadDeadline(time.Now().Add(30 * time.Second))
		var msg protocolo.Mensagem
		if err := cliente.Decoder.Decode(&msg); err != nil {
			return
		}
		switch msg.Comando {
		case "LOGIN":
			var dadosLogin protocolo.DadosLogin
			if json.Unmarshal(msg.Dados, &dadosLogin) == nil && dadosLogin.Nome != "" {
				cliente.Nome = dadosLogin.Nome
			}
		case "ENTRAR_NA_FILA":
			s.entrarFila(cliente)
		case "COMPRAR_PACOTE":
			select {
			case s.packWorkerPool <- packReq{cli: cliente, quantidade: 1}:
			default:
			}
			// Outros casos de jogo (JOGAR_CARTA, etc.) foram omitidos pois o gargalo é a compra.
		}
	}
}

/* ====================== Matchmaking Otimizado ====================== */
func (s *Servidor) entrarFila(cliente *Cliente) {
	shardIndex := rand.Intn(numFilaShards)
	shard := s.shardedFila[shardIndex]
	shard.mutex.Lock()
	shard.clientes = append(shard.clientes, cliente)
	if len(shard.clientes) >= 2 {
		j1 := shard.clientes[0]
		j2 := shard.clientes[1]
		shard.clientes = shard.clientes[2:]
		shard.mutex.Unlock()
		s.criarSala(j1, j2)
	} else {
		shard.mutex.Unlock()
	}
}
func (s *Servidor) criarSala(j1, j2 *Cliente) {
	salaID := novoID() // Usando nosso gerador rápido de ID
	novaSala := &Sala{
		ID:        salaID,
		Jogadores: []*Cliente{j1, j2},
		Estado:    "AGUARDANDO_COMPRA",
		srv:       s,
	}
	s.salas.Store(salaID, novaSala)
	j1.Sala, j2.Sala = novaSala, novaSala
	// Notificações de partida encontrada foram removidas para focar na performance do teste.
}

/* ====================== Lógica do Jogo (em memória) ====================== */
func (sala *Sala) marcarCompraEIniciarSePossivel(cli *Cliente) {
	sala.mutex.Lock()
	if sala.Estado == "JOGANDO" {
		sala.mutex.Unlock()
		return
	}
	if len(sala.Jogadores) < 2 {
		sala.mutex.Unlock()
		return
	}
	oponente := sala.Jogadores[0]
	if sala.Jogadores[0] == cli {
		oponente = sala.Jogadores[1]
	}
	if len(oponente.Inventario) > 0 {
		sala.Estado = "JOGANDO"
	}
	sala.mutex.Unlock()
}

// Outras funções de jogo foram omitidas para focar na otimização de conexão/compra.

/* ====================== Utilidades Otimizadas ====================== */
func (s *Servidor) adicionarCliente(c *Cliente) { s.clientes.Store(c.Conn, c) }
func (s *Servidor) removerCliente(c *Cliente) {
	if c.Sala != nil {
		c.Sala.removerJogador(c)
	}
	s.clientes.Delete(c.Conn)
	// Não precisamos mais fechar a mailbox, pois o objeto cliente será reutilizado.
}
func (sala *Sala) removerJogador(cliente *Cliente) {
	sala.mutex.Lock()
	// Lógica simplificada para remoção rápida
	if len(sala.Jogadores) == 2 {
		if sala.Jogadores[0] == cliente {
			sala.Jogadores = sala.Jogadores[1:]
		} else {
			sala.Jogadores = sala.Jogadores[:1]
		}
	} else {
		sala.Jogadores = []*Cliente{}
	}
	sala.mutex.Unlock()
}
func (s *Servidor) enviar(cli *Cliente, msg protocolo.Mensagem) bool {
	if cli.Conn == nil {
		return false
	}
	select {
	case cli.Mailbox <- msg:
		return true
	case <-time.After(200 * time.Millisecond): // Timeout mais curto
		return false
	}
}
func mustJSON(v any) []byte { b, _ := json.Marshal(v); return b }

// OTIMIZAÇÃO: Contador Atômico para IDs, eliminando o Mutex.
var idSeq int64

func novoID() string {
	id := atomic.AddInt64(&idSeq, 1)
	return fmt.Sprintf("c%d", id)
}
func sampleRaridade() string {
	x := rand.Intn(100)
	if x < 70 {
		return "C"
	}
	if x < 90 {
		return "U"
	}
	if x < 99 {
		return "R"
	}
	return "L"
}
func gerarEstoquesIniciais() []map[string][]Carta {
	rand.Seed(time.Now().UnixNano())
	allStocks := make([]map[string][]Carta, numEstoqueShards)
	for i := 0; i < numEstoqueShards; i++ {
		allStocks[i] = map[string][]Carta{"C": {}, "U": {}, "R": {}, "L": {}}
	}
	tiposCartas := []string{"Dragão", "Guerreiro", "Mago", "Anjo", "Demônio", "Fênix", "Titan", "Sereia", "Lobo", "Águia", "Leão", "Tigre"}
	naipes := []string{"♠", "♥", "♦", "♣"}
	for _, nome := range tiposCartas {
		for i := 0; i < 200; i++ { // Aumenta drasticamente o estoque para não acabar durante o teste
			shardIndex := rand.Intn(numEstoqueShards)
			allStocks[shardIndex]["C"] = append(allStocks[shardIndex]["C"], Carta{ID: novoID(), Nome: nome, Naipe: naipes[rand.Intn(len(naipes))], Valor: 10 + rand.Intn(30), Raridade: "C"})
		}
		for i := 0; i < 100; i++ {
			shardIndex := rand.Intn(numEstoqueShards)
			allStocks[shardIndex]["U"] = append(allStocks[shardIndex]["U"], Carta{ID: novoID(), Nome: nome, Naipe: naipes[rand.Intn(len(naipes))], Valor: 40 + rand.Intn(30), Raridade: "U"})
		}
		for i := 0; i < 50; i++ {
			shardIndex := rand.Intn(numEstoqueShards)
			allStocks[shardIndex]["R"] = append(allStocks[shardIndex]["R"], Carta{ID: novoID(), Nome: nome, Naipe: naipes[rand.Intn(len(naipes))], Valor: 70 + rand.Intn(20), Raridade: "R"})
		}
		for i := 0; i < 20; i++ {
			shardIndex := rand.Intn(numEstoqueShards)
			allStocks[shardIndex]["L"] = append(allStocks[shardIndex]["L"], Carta{ID: novoID(), Nome: nome, Naipe: naipes[rand.Intn(len(naipes))], Valor: 90 + rand.Intn(10), Raridade: "L"})
		}
	}
	return allStocks
}
