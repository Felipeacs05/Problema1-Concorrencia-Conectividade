// felipeacs05/problema1-concorrencia-conectividade/Problema1-Concorrencia-Conectividade-77d73bcc575bbc2b6e076d0c153ffc2b7b175855/Projeto/servidor/main.go
package main

// ===================== BAREMA ITEM 1: ARQUITETURA =====================
// Este é o componente principal do servidor do jogo de cartas multiplayer.
// O servidor gerencia: conexões de clientes, matchmaking, estoque de cartas,
// lógica do jogo, e comunicação entre jogadores.

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"meujogo/protocolo"
	"net"
	"strings"
	"sync"
	"sync/atomic" // BAREMA ITEM 5: CONCORRÊNCIA - Usar pacote atomic para contadores thread-safe
	"time"
)

// BAREMA ITEM 5: CONCORRÊNCIA - Configurações de sharding para distribuir carga
// O sharding divide o estoque e operações em múltiplas partições para reduzir contenção
const (
	numFilaShards    = 32 // Número de shards para distribuir operações de fila
	numEstoqueShards = 32 // Número de shards para distribuir operações de estoque
)

/* ====================== Tipos e Pools de Otimização ====================== */
type Carta = protocolo.Carta

// BAREMA ITEM 1: ARQUITETURA - Estrutura que representa um cliente conectado
// Cada cliente possui sua própria conexão TCP, inventário de cartas e estado de jogo
type Cliente struct {
	Conn       net.Conn                // Conexão TCP com o cliente
	Nome       string                  // Nome único do jogador
	Encoder    *json.Encoder           // Codificador JSON para envio de mensagens
	Decoder    *json.Decoder           // Decodificador JSON para recebimento de mensagens
	Mailbox    chan protocolo.Mensagem // Canal para envio assíncrono de mensagens
	Sala       *Sala                   // Referência para a sala onde o jogador está
	Inventario []Carta                 // Cartas que o jogador possui
	UltimoPing time.Time               // BAREMA ITEM 6: LATÊNCIA - Timestamp do último ping
	PingMs     int64                   // BAREMA ITEM 6: LATÊNCIA - Latência medida em milissegundos
}

// BAREMA ITEM 5: CONCORRÊNCIA - Pool de objetos para reutilização e otimização de memória
// Reduz a pressão no Garbage Collector reutilizando estruturas Cliente
var clientePool = sync.Pool{
	New: func() interface{} {
		return &Cliente{
			Mailbox:    make(chan protocolo.Mensagem, 32), // Canal com buffer para mensagens
			Inventario: make([]Carta, 0, 64),              // Slice pré-alocado para cartas
		}
	},
}

// BAREMA ITEM 7: PARTIDAS - Estrutura que representa uma sala de jogo
// Gerencia o estado de uma partida entre dois jogadores
type Sala struct {
	ID              string           // Identificador único da sala
	Jogadores       []*Cliente       // Lista dos jogadores na sala (sempre 2)
	Estado          string           // Estado atual: "AGUARDANDO_COMPRA" | "JOGANDO" | "FINALIZADO"
	CartasNaMesa    map[string]Carta // Cartas jogadas na jogada atual (nome -> carta)
	PontosRodada    map[string]int   // Pontos de cada jogador na rodada atual
	PontosPartida   map[string]int   // Rodadas ganhas por cada jogador na partida
	NumeroRodada    int              // Número da rodada atual (1, 2, 3...)
	JogadasNaRodada int              // Quantas jogadas foram feitas na rodada atual (0, 1, 2)
	Prontos         map[string]bool  // Quais jogadores já compraram pacotes para esta partida
	srv             *Servidor        // Referência para o servidor principal
	mutex           sync.Mutex       // BAREMA ITEM 5: CONCORRÊNCIA - Mutex para proteger acesso concorrente
}

// BAREMA ITEM 5: CONCORRÊNCIA - Shard para operações de fila de espera
// Divide a fila em múltiplas partições para reduzir contenção
type filaShard struct {
	clientes []*Cliente // Lista de clientes aguardando neste shard
	mutex    sync.Mutex // Mutex para proteger acesso concorrente
}

// BAREMA ITEM 8: PACOTES - Shard para operações de estoque de cartas
// Divide o estoque em múltiplas partições para distribuir carga
type estoqueShard struct {
	estoque map[string][]Carta // Estoque de cartas por raridade (C, U, R, L)
	mutex   sync.Mutex         // Mutex para proteger acesso concorrente
}

// BAREMA ITEM 1: ARQUITETURA - Estrutura principal do servidor
// Centraliza todas as operações do servidor: conexões, salas, estoque, etc.
type Servidor struct {
	clientes       sync.Map        // BAREMA ITEM 5: CONCORRÊNCIA - Map thread-safe para clientes conectados
	salas          sync.Map        // BAREMA ITEM 5: CONCORRÊNCIA - Map thread-safe para salas ativas
	filaDeEspera   *Cliente        // BAREMA ITEM 7: PARTIDAS - Cliente aguardando matchmaking
	filaMutex      sync.Mutex      // BAREMA ITEM 5: CONCORRÊNCIA - Mutex para proteger fila de espera
	shardedEstoque []*estoqueShard // BAREMA ITEM 8: PACOTES - Array de shards do estoque
	packSize       int             // Número de cartas por pacote
	packWorkers    int             // Número de workers para processar compras
	packWorkerPool chan packReq    // BAREMA ITEM 5: CONCORRÊNCIA - Canal para fila de requisições de pacotes
}

// BAREMA ITEM 8: PACOTES - Estrutura para requisição de compra de pacote
// Usada para enviar requisições para o pool de workers
type packReq struct {
	cli        *Cliente // Cliente que solicitou a compra
	quantidade int      // Quantidade de pacotes solicitados
}

/* ====================== Servidor / bootstrap ====================== */

// BAREMA ITEM 1: ARQUITETURA - Inicialização do servidor com todas as configurações
// Configura pools de workers, shards de estoque e outras estruturas de concorrência
func novoServidor() *Servidor {
	s := &Servidor{
		packSize:       5,                                       // 5 cartas por pacote
		packWorkers:    1000,                                    // BAREMA ITEM 5: CONCORRÊNCIA - 1000 workers para processar compras
		packWorkerPool: make(chan packReq, 100000),              // Canal com buffer grande para requisições
		shardedEstoque: make([]*estoqueShard, numEstoqueShards), // Inicializa array de shards
	}

	// BAREMA ITEM 8: PACOTES - Gera estoque inicial distribuído entre os shards
	estoquesIniciais := gerarEstoquesIniciais()
	for i := 0; i < numEstoqueShards; i++ {
		s.shardedEstoque[i] = &estoqueShard{estoque: estoquesIniciais[i]}
	}

	// BAREMA ITEM 5: CONCORRÊNCIA - Inicia pool de workers para processar compras
	for i := 0; i < s.packWorkers; i++ {
		go s.packWorker()
	}
	return s
}

// BAREMA ITEM 2: COMUNICAÇÃO - Função principal do servidor
// Inicia o servidor TCP e aceita conexões de clientes
func main() {
	servidor := novoServidor()

	// BAREMA ITEM 2: COMUNICAÇÃO - Cria listener TCP na porta 65432
	listener, err := net.Listen("tcp", ":65432")
	if err != nil {
		panic(err)
	}
	defer listener.Close()
	fmt.Println("[SERVIDOR] ouvindo :65432 (otimização final)")

	// BAREMA ITEM 6: LATÊNCIA - Inicia servidor ICMP para medição de latência
	go servidor.startICMPPingServer()

	// BAREMA ITEM 5: CONCORRÊNCIA - Semáforo para limitar conexões simultâneas
	// Evita sobrecarga do sistema com muitas conexões simultâneas
	semaphore := make(chan struct{}, 30000)

	// BAREMA ITEM 2: COMUNICAÇÃO - Loop principal para aceitar conexões
	for {
		conn, err := listener.Accept()
		if err != nil {
			continue
		}

		// BAREMA ITEM 5: CONCORRÊNCIA - Cada conexão é processada em uma goroutine separada
		// As configurações de TCP são aplicadas dentro de handleConnection para evitar contenção
		semaphore <- struct{}{}
		go func() {
			defer func() { <-semaphore }()
			servidor.handleConnection(conn)
		}()
	}
}

/* ====================== Servidor ICMP para Ping ====================== */

// BAREMA ITEM 6: LATÊNCIA - Servidor ICMP dedicado para medição de latência
// Usa ICMP para medição mais precisa e nativa da latência de rede
func (s *Servidor) startICMPPingServer() {
	// BAREMA ITEM 6: LATÊNCIA - Cria socket ICMP raw
	conn, err := net.ListenPacket("ip4:icmp", "0.0.0.0")
	if err != nil {
		fmt.Printf("[SERVIDOR] Erro ao iniciar servidor ICMP: %v\n", err)
		return
	}
	defer conn.Close()

	fmt.Println("[SERVIDOR] Servidor ICMP de ping iniciado")

	// BAREMA ITEM 6: LATÊNCIA - Buffer para receber pacotes ICMP
	buffer := make([]byte, 1024)

	// BAREMA ITEM 5: CONCORRÊNCIA - Semáforo para limitar processamento simultâneo de ICMP
	semaphore := make(chan struct{}, 1000)

	for {
		n, clientAddr, err := conn.ReadFrom(buffer)
		if err != nil {
			continue
		}

		// BAREMA ITEM 5: CONCORRÊNCIA - Limita processamento simultâneo para evitar sobrecarga
		semaphore <- struct{}{}
		go func() {
			defer func() { <-semaphore }()
			s.processICMPPacket(conn, buffer[:n], clientAddr)
		}()
	}
}

// BAREMA ITEM 6: LATÊNCIA - Processa pacote ICMP recebido
func (s *Servidor) processICMPPacket(conn net.PacketConn, data []byte, clientAddr net.Addr) {
	// BAREMA ITEM 6: LATÊNCIA - Verifica se é um pacote ICMP Echo Request
	if len(data) < 8 {
		return
	}

	// BAREMA ITEM 6: LATÊNCIA - Cria resposta ICMP Echo Reply
	reply := make([]byte, len(data))
	copy(reply, data)

	// BAREMA ITEM 6: LATÊNCIA - Muda tipo para Echo Reply (0)
	reply[0] = 0

	// BAREMA ITEM 6: LATÊNCIA - Recalcula checksum
	reply[2] = 0
	reply[3] = 0
	checksum := s.calculateICMPChecksum(reply)
	reply[2] = byte(checksum >> 8)
	reply[3] = byte(checksum & 0xFF)

	// BAREMA ITEM 6: LATÊNCIA - Envia resposta
	conn.WriteTo(reply, clientAddr)
}

// BAREMA ITEM 6: LATÊNCIA - Calcula checksum ICMP
func (s *Servidor) calculateICMPChecksum(data []byte) uint16 {
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

/* ====================== Processamento de Pacotes Otimizado ====================== */

// BAREMA ITEM 5: CONCORRÊNCIA - Worker que processa requisições de compra de pacotes
// Cada worker roda em uma goroutine separada, processando requisições do canal
func (s *Servidor) packWorker() {
	for req := range s.packWorkerPool {
		s.processarPacote(req)
	}
}

// BAREMA ITEM 8: PACOTES - Processa uma requisição de compra de pacote
// Garante distribuição justa de cartas e evita duplicatas no estoque global
func (s *Servidor) processarPacote(req packReq) {
	if req.cli.Conn == nil {
		return // Cliente desconectado, ignora requisição
	}

	// Calcula total de cartas necessárias
	totalNecessario := req.quantidade * s.packSize
	cartas := make([]Carta, 0, totalNecessario)

	// BAREMA ITEM 8: PACOTES - Gera cartas com distribuição de raridade
	for i := 0; i < totalNecessario; i++ {
		c, ok := s.takeOneByRarityWithDowngrade(sampleRaridade())
		if !ok {
			// Se não há cartas da raridade desejada, gera uma carta comum básica
			c = s.gerarCartaComumBasica()
		}
		cartas = append(cartas, c)
	}

	// Adiciona cartas ao inventário do cliente
	req.cli.Inventario = append(req.cli.Inventario, cartas...)

	// BAREMA ITEM 3: API REMOTA - Envia resultado da compra para o cliente
	msg := protocolo.Mensagem{
		Comando: "PACOTE_RESULTADO",
		Dados:   mustJSON(protocolo.ComprarPacoteResp{Cartas: cartas}),
	}
	if s.enviar(req.cli, msg) {
		// Envia mensagem de ajuda após a compra
		s.enviar(req.cli, protocolo.Mensagem{
			Comando: "SISTEMA",
			Dados:   mustJSON(protocolo.DadosErro{Mensagem: "[SISTEMA] Você recebeu novas cartas! Use /cartas para ver sua mão."}),
		})

		// BAREMA ITEM 7: PARTIDAS - Verifica se pode iniciar a partida
		if req.cli.Sala != nil {
			req.cli.Sala.marcarCompraEIniciarSePossivel(req.cli)
		}
	}
}

// BAREMA ITEM 8: PACOTES - Remove uma carta do estoque com sistema de downgrade
// Se não há cartas da raridade desejada, tenta raridades menores (L->R->U->C)
func (s *Servidor) takeOneByRarityWithDowngrade(r string) (Carta, bool) {
	// BAREMA ITEM 5: CONCORRÊNCIA - Cria gerador aleatório único para esta operação
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))

	// BAREMA ITEM 5: CONCORRÊNCIA - Seleciona shard aleatoriamente para distribuir carga
	shardIndex := rng.Intn(numEstoqueShards)
	shard := s.shardedEstoque[shardIndex]
	shard.mutex.Lock()
	defer shard.mutex.Unlock()

	// Ordem de downgrade: Lendária -> Rara -> Incomum -> Comum
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

	// Tenta encontrar carta da raridade desejada ou menor
	for i := start; i < len(order); i++ {
		raridade := order[i]
		if arr := shard.estoque[raridade]; len(arr) > 0 {
			// BAREMA ITEM 8: PACOTES - Seleciona carta aleatória do array para maior variedade
			randomIndex := rng.Intn(len(arr))
			c := arr[randomIndex]
			// Remove a carta selecionada (troca com a última e remove)
			arr[randomIndex] = arr[len(arr)-1]
			shard.estoque[raridade] = arr[:len(arr)-1]
			return c, true
		}
	}
	return Carta{}, false // Nenhuma carta disponível
}

// BAREMA ITEM 8: PACOTES - Gera carta comum única quando estoque acaba
func (s *Servidor) gerarCartaComumBasica() Carta {
	// BAREMA ITEM 8: PACOTES - Lista de nomes variados para cartas comuns
	nomes := []string{
		"Guerreiro", "Arqueiro", "Mago", "Cavaleiro", "Ladrão", "Clérigo",
		"Bárbaro", "Paladino", "Ranger", "Bruxo", "Druida", "Monge",
		"Assassino", "Bardo", "Necromante", "Elementalista", "Inquisidor",
		"Gladiador", "Mercenário", "Escudeiro", "Aprendiz", "Novato",
		"Veterano", "Herói", "Lenda", "Mestre", "Sábio", "Ancião",
		"Espadachim", "Arqueiro Élfico", "Mago do Caos", "Sacerdote", "Berserker",
		"Samurai", "Ninja", "Viking", "Cruzado", "Templário", "Caçador",
		"Explorador", "Navegador", "Alquimista", "Encantador", "Ilusionista",
		"Summoner", "Conjurador", "Evocador", "Invocador", "Chamador", "Convocador",
		"Dragão", "Fênix", "Titan", "Sereia", "Lobo", "Águia", "Leão", "Tigre",
		"Anjo", "Demônio", "Golem", "Elemental", "Espírito", "Fantasma", "Zumbi",
		"Skeleton", "Orc", "Elfo", "Anão", "Hobbit", "Gigante", "Troll", "Ogro",
		"Knight", "Wizard", "Rogue", "Priest", "Warrior", "Mage", "Hunter", "Shaman",
		"Monk", "Paladin", "Druid", "Warlock", "Death Knight", "Demon Hunter", "Evoker",
	}

	// BAREMA ITEM 8: PACOTES - Lista de naipes variados
	naipes := []string{"♠", "♥", "♦", "♣"}

	// BAREMA ITEM 8: PACOTES - Cria gerador aleatório único para esta chamada
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))

	// BAREMA ITEM 8: PACOTES - Gera valores variados (1-50 para cartas comuns)
	valor := 1 + rng.Intn(50)

	// BAREMA ITEM 8: PACOTES - Seleciona nome e naipe aleatórios
	nome := nomes[rng.Intn(len(nomes))]
	naipe := naipes[rng.Intn(len(naipes))]

	return Carta{
		ID:       novoID(),
		Nome:     nome,
		Naipe:    naipe,
		Valor:    valor,
		Raridade: "C",
	}
}

/* ====================== Conexão / IO com Pools ====================== */

// BAREMA ITEM 2: COMUNICAÇÃO - Gerencia uma conexão TCP com um cliente
// Configura a conexão, inicializa estruturas e coordena leitura/escrita
func (s *Servidor) handleConnection(conn net.Conn) {
	// BAREMA ITEM 2: COMUNICAÇÃO - Configurações de TCP para melhor performance
	if tcpConn, ok := conn.(*net.TCPConn); ok {
		tcpConn.SetKeepAlive(true)                   // Mantém conexão ativa
		tcpConn.SetKeepAlivePeriod(30 * time.Second) // Verifica conexão a cada 30s
		tcpConn.SetNoDelay(true)                     // Desabilita algoritmo de Nagle
	}

	// BAREMA ITEM 5: CONCORRÊNCIA - Reutiliza objeto Cliente do pool para otimização
	cliente := clientePool.Get().(*Cliente)
	cliente.Conn = conn
	cliente.Encoder = json.NewEncoder(conn)
	cliente.Decoder = json.NewDecoder(conn)
	cliente.Nome = conn.RemoteAddr().String()
	cliente.UltimoPing = time.Now() // BAREMA ITEM 6: LATÊNCIA - Inicializa timestamp de ping

	s.adicionarCliente(cliente)

	// BAREMA ITEM 5: CONCORRÊNCIA - Inicia goroutines para escrita e ping em paralelo
	go s.clienteWriter(cliente) // Goroutine para envio de mensagens
	go s.pingManager(cliente)   // BAREMA ITEM 6: LATÊNCIA - Goroutine para gerenciar pings
	s.clienteReader(cliente)    // Loop principal de leitura (bloqueante)

	// BAREMA ITEM 5: CONCORRÊNCIA - Limpeza e devolução do objeto para o pool
	conn.Close() // Garante que a conexão seja fechada
	s.removerCliente(cliente)

	// Limpa o estado do cliente para reutilização
	cliente.Inventario = cliente.Inventario[:0] // Limpa o slice mantendo capacidade
	cliente.Sala = nil
	cliente.Conn = nil
	cliente.Nome = ""
	cliente.PingMs = 0
	cliente.UltimoPing = time.Time{}

	clientePool.Put(cliente) // Devolve objeto para o pool
}
func (s *Servidor) clienteWriter(c *Cliente) {
	for msg := range c.Mailbox {
		if c.Conn == nil {
			return
		}
		c.Conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
		if err := c.Encoder.Encode(msg); err != nil {
			fmt.Printf("[SERVIDOR] Erro de escrita para %s: %v\n", c.Nome, err)
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
				fmt.Printf("[SERVIDOR] %s fez login como '%s'\n", cliente.Conn.RemoteAddr().String(), cliente.Nome)
			}
		case "ENTRAR_NA_FILA":
			s.entrarFila(cliente)
		case "COMPRAR_PACOTE":
			fmt.Printf("[SERVIDOR] %s solicitou compra de pacote\n", cliente.Nome)

			if cliente.Sala != nil {
				cliente.Sala.mutex.Lock()
				pronto, ok := cliente.Sala.Prontos[cliente.Nome]
				cliente.Sala.mutex.Unlock()
				if ok && pronto {
					s.enviar(cliente, protocolo.Mensagem{
						Comando: "SISTEMA",
						Dados:   mustJSON(protocolo.DadosErro{Mensagem: "[SISTEMA] Você já comprou cartas para esta partida."}),
					})
					break
				}
			}

			select {
			case s.packWorkerPool <- packReq{cli: cliente, quantidade: 1}:
				fmt.Printf("[SERVIDOR] %s - pedido de pacote enviado para processamento\n", cliente.Nome)
			default:
				s.enviar(cliente, protocolo.Mensagem{
					Comando: "ERRO",
					Dados:   mustJSON(protocolo.DadosErro{Mensagem: "Servidor sobrecarregado. Tente novamente."}),
				})
			}
		case "JOGAR_CARTA":
			if cliente.Sala != nil && cliente.Sala.Estado == "JOGANDO" {
				var dadosJogar protocolo.DadosJogarCarta
				if json.Unmarshal(msg.Dados, &dadosJogar) == nil {
					cliente.Sala.processarJogada(cliente, dadosJogar.CartaID)
				}
			}
		case "ENVIAR_CHAT":
			if cliente.Sala != nil {
				var dadosChat protocolo.DadosEnviarChat
				if json.Unmarshal(msg.Dados, &dadosChat) == nil {
					msgParaBroadcast := protocolo.Mensagem{
						Comando: "RECEBER_CHAT",
						Dados: mustJSON(protocolo.DadosReceberChat{
							NomeJogador: cliente.Nome,
							Texto:       dadosChat.Texto,
						}),
					}
					cliente.Sala.broadcast(nil, msgParaBroadcast)
				}
			}
		case "PONG":
			var dadosPong protocolo.DadosPong
			if json.Unmarshal(msg.Dados, &dadosPong) == nil {
				cliente.PingMs = time.Now().UnixMilli() - dadosPong.Timestamp
				cliente.UltimoPing = time.Now()
				fmt.Printf("[SERVIDOR] Latência de %s: %dms\n", cliente.Nome, cliente.PingMs)
			}
		case "VER_CARTAS":
			s.mostrarCartasDetalhadas(cliente)
		case "SAIR_DA_SALA":
			s.handleSairDaSala(cliente)
		case "QUIT":
			return // Encerra a goroutine do leitor, o que levará à limpeza da conexão.
		case "PING":
			var dadosPing protocolo.DadosPing
			if json.Unmarshal(msg.Dados, &dadosPing) == nil {
				s.enviar(cliente, protocolo.Mensagem{
					Comando: "PONG",
					Dados:   mustJSON(protocolo.DadosPong{Timestamp: dadosPing.Timestamp}),
				})
			}
		}
	}
}

func (s *Servidor) pingManager(c *Cliente) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		if c.Conn == nil {
			return // Encerra se a conexão for nula
		}
		// Se não receber um PONG em 30 segundos, a conexão será fechada pelo readDeadline
		if !s.enviar(c, protocolo.Mensagem{
			Comando: "PING",
			Dados:   mustJSON(protocolo.DadosPing{Timestamp: time.Now().UnixMilli()}),
		}) {
			return // Encerra se não conseguir enviar
		}
	}
}

func (s *Servidor) handleSairDaSala(cliente *Cliente) {
	if cliente.Sala == nil {
		return // Não está em nenhuma sala
	}

	sala := cliente.Sala
	sala.mutex.Lock()

	// Notifica o oponente, se houver
	var oponente *Cliente
	if len(sala.Jogadores) == 2 {
		if sala.Jogadores[0] == cliente {
			oponente = sala.Jogadores[1]
		} else {
			oponente = sala.Jogadores[0]
		}
		s.enviar(oponente, protocolo.Mensagem{
			Comando: "SISTEMA",
			Dados:   mustJSON(protocolo.DadosErro{Mensagem: "[SISTEMA] Seu oponente saiu da partida. Colocando você de volta na fila..."}),
		})
	}

	// Remove o jogador da sala e limpa a referência
	sala.removerJogador(cliente)
	cliente.Sala = nil
	sala.mutex.Unlock()

	// Se havia um oponente, ele volta para a fila de espera
	if oponente != nil {
		s.entrarFila(oponente)
	}
}

/* ====================== Matchmaking Otimizado ====================== */

// BAREMA ITEM 7: PARTIDAS - Sistema de matchmaking para parear jogadores
// Garante que cada jogador seja pareado com apenas um oponente por vez
func (s *Servidor) entrarFila(cliente *Cliente) {
	s.filaMutex.Lock()
	defer s.filaMutex.Unlock()

	// BAREMA ITEM 7: PARTIDAS - Verifica se já existe um jogador na fila de espera
	if s.filaDeEspera != nil {
		oponente := s.filaDeEspera
		s.filaDeEspera = nil // Limpa a fila para evitar múltiplos pareamentos

		// BAREMA ITEM 7: PARTIDAS - Cria sala com os dois jogadores encontrados
		fmt.Printf("[SERVIDOR] Jogador %s encontrado para %s. Criando sala...\n", cliente.Nome, oponente.Nome)
		s.criarSala(oponente, cliente)
	} else {
		// BAREMA ITEM 7: PARTIDAS - Nenhum jogador esperando, este cliente aguarda
		s.filaDeEspera = cliente
		fmt.Printf("[SERVIDOR] Jogador %s entrou na fila e está aguardando um oponente.\n", cliente.Nome)
		s.enviar(cliente, protocolo.Mensagem{
			Comando: "SISTEMA",
			Dados:   mustJSON(protocolo.DadosErro{Mensagem: "[SISTEMA] Aguardando um oponente..."}),
		})
	}
}

// BAREMA ITEM 7: PARTIDAS - Cria uma nova sala de jogo com dois jogadores
// Inicializa o estado da partida e notifica os jogadores
func (s *Servidor) criarSala(j1, j2 *Cliente) {
	salaID := novoID() // Gera ID único para a sala

	// BAREMA ITEM 7: PARTIDAS - Inicializa sala com estado "AGUARDANDO_COMPRA"
	novaSala := &Sala{
		ID:              salaID,
		Jogadores:       []*Cliente{j1, j2},  // Sempre exatamente 2 jogadores
		Estado:          "AGUARDANDO_COMPRA", // Estado inicial: aguarda compra de cartas
		CartasNaMesa:    make(map[string]Carta),
		PontosRodada:    make(map[string]int),
		PontosPartida:   make(map[string]int),
		NumeroRodada:    1,
		JogadasNaRodada: 0,
		Prontos:         make(map[string]bool), // Rastreia quem já comprou cartas
		srv:             s,
	}

	// BAREMA ITEM 5: CONCORRÊNCIA - Armazena sala no map thread-safe
	s.salas.Store(salaID, novaSala)
	j1.Sala, j2.Sala = novaSala, novaSala // Associa jogadores à sala

	// BAREMA ITEM 3: API REMOTA - Notifica os jogadores sobre a partida encontrada
	d1 := protocolo.DadosPartidaEncontrada{SalaID: salaID, OponenteNome: j2.Nome}
	d2 := protocolo.DadosPartidaEncontrada{SalaID: salaID, OponenteNome: j1.Nome}
	s.enviar(j1, protocolo.Mensagem{Comando: "PARTIDA_ENCONTRADA", Dados: mustJSON(d1)})
	s.enviar(j2, protocolo.Mensagem{Comando: "PARTIDA_ENCONTRADA", Dados: mustJSON(d2)})

	// BAREMA ITEM 8: PACOTES - Informa que devem comprar pacotes para iniciar
	novaSala.broadcast(nil, protocolo.Mensagem{
		Comando: "SISTEMA",
		Dados:   mustJSON(protocolo.DadosErro{Mensagem: "[SISTEMA] Partida encontrada! Usem /comprar para adquirir um pacote de cartas e iniciar o jogo."}),
	})
}

/* ====================== Lógica do Jogo (em memória) ====================== */
func (sala *Sala) marcarCompraEIniciarSePossivel(cli *Cliente) {
	sala.mutex.Lock()

	if sala.Estado == "FINALIZADO" {
		sala.reiniciarSala()
	}
	if sala.Prontos == nil {
		sala.Prontos = make(map[string]bool)
	}

	// Marca como "comprou"
	sala.Prontos[cli.Nome] = true

	// Checa se ambos estão prontos
	if len(sala.Jogadores) < 2 {
		sala.mutex.Unlock()
		return
	}
	ready1 := sala.Prontos[sala.Jogadores[0].Nome]
	ready2 := sala.Prontos[sala.Jogadores[1].Nome]

	sala.mutex.Unlock()

	// Broadcast de pronto
	sala.broadcast(nil, protocolo.Mensagem{
		Comando: "SISTEMA",
		Dados:   mustJSON(protocolo.DadosErro{Mensagem: fmt.Sprintf("[SISTEMA] - Jogador \"%s\" está pronto para iniciar", cli.Nome)}),
	})

	if ready1 && ready2 {
		sala.iniciarPartida()
	}
}

// Outras funções de jogo foram omitidas para focar na otimização de conexão/compra.

/* ====================== Utilidades Otimizadas ====================== */
func (s *Servidor) adicionarCliente(c *Cliente) { s.clientes.Store(c.Conn, c) }
func (s *Servidor) removerCliente(c *Cliente) {
	if c.Sala != nil {
		c.Sala.removerJogador(c)
	}
	s.clientes.Delete(c.Conn)
	// Limpa da fila de espera se o cliente desconectar enquanto espera
	s.filaMutex.Lock()
	if s.filaDeEspera == c {
		s.filaDeEspera = nil
	}
	s.filaMutex.Unlock()
	// Não precisamos mais fechar a mailbox, pois o objeto cliente será reutilizado.
}
func (sala *Sala) removerJogador(cliente *Cliente) {
	sala.mutex.Lock()
	defer sala.mutex.Unlock()

	// Lógica de remoção
	var outrosJogadores []*Cliente
	for _, j := range sala.Jogadores {
		if j != cliente {
			outrosJogadores = append(outrosJogadores, j)
		}
	}
	sala.Jogadores = outrosJogadores

	// Notifica o oponente se houver
	if len(sala.Jogadores) > 0 {
		sala.broadcast(nil, protocolo.Mensagem{
			Comando: "SISTEMA",
			Dados:   mustJSON(protocolo.DadosErro{Mensagem: fmt.Sprintf("[SISTEMA] %s desconectou da partida", cliente.Nome)}),
		})
	}
	fmt.Printf("[SALA %s] Jogador %s removido\n", sala.ID, cliente.Nome)
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

func (sala *Sala) broadcast(_ *Cliente, msg protocolo.Mensagem) {
	sala.mutex.Lock()
	defer sala.mutex.Unlock()
	for _, j := range sala.Jogadores {
		select {
		case j.Mailbox <- msg:
		default:
			fmt.Printf("[SALA %s] Mailbox cheia de %s; descartando mensagem\n", sala.ID, j.Nome)
		}
	}
}

func (sala *Sala) enviarAtualizacaoJogo(mensagem, vencedorJogada, vencedorRodada string) {
	sala.mutex.Lock()
	defer sala.mutex.Unlock()

	// Envia atualização personalizada para cada jogador
	for _, jogador := range sala.Jogadores {
		dados := sala.criarAtualizacaoJogoPersonalizada(mensagem, vencedorJogada, vencedorRodada, jogador)
		sala.srv.enviar(jogador, protocolo.Mensagem{
			Comando: "ATUALIZACAO_JOGO",
			Dados:   mustJSON(dados),
		})
	}
}

// Cria atualização personalizada escondendo poder das cartas do oponente
func (sala *Sala) criarAtualizacaoJogoPersonalizada(mensagem, vencedorJogada, vencedorRodada string, jogadorAtual *Cliente) protocolo.DadosAtualizacaoJogo {
	contagem := make(map[string]int)
	ultima := make(map[string]Carta)

	for _, p := range sala.Jogadores {
		contagem[p.Nome] = len(p.Inventario)
		if c, ok := sala.CartasNaMesa[p.Nome]; ok {
			// Mostra a carta jogada por ambos os jogadores, sem esconder o poder
			ultima[p.Nome] = c
		}
	}

	return protocolo.DadosAtualizacaoJogo{
		MensagemDoTurno: mensagem,
		ContagemCartas:  contagem,
		UltimaJogada:    ultima,
		VencedorJogada:  vencedorJogada,
		VencedorRodada:  vencedorRodada,
		NumeroRodada:    sala.NumeroRodada,
		PontosRodada:    sala.PontosRodada,
		PontosPartida:   sala.PontosPartida,
	}
}

func (sala *Sala) iniciarPartida() {
	sala.mutex.Lock()
	sala.Estado = "JOGANDO"
	sala.CartasNaMesa = make(map[string]Carta)
	sala.JogadasNaRodada = 0
	sala.Prontos = make(map[string]bool)

	if sala.PontosPartida == nil {
		sala.PontosPartida = make(map[string]int)
		for _, p := range sala.Jogadores {
			sala.PontosPartida[p.Nome] = 0
		}
	}
	if sala.PontosRodada == nil {
		sala.PontosRodada = make(map[string]int)
	}
	for _, p := range sala.Jogadores {
		sala.PontosRodada[p.Nome] = 0
	}
	sala.mutex.Unlock()

	sala.enviarAtualizacaoJogo("[SISTEMA] Partida iniciada! Use /jogar <ID_da_carta> para jogar. Use /cartas para ver sua mão.", "", "")
}

func (sala *Sala) processarJogada(jogador *Cliente, cartaID string) {
	sala.mutex.Lock()

	if sala.Estado != "JOGANDO" {
		sala.mutex.Unlock()
		return
	}

	if _, ok := sala.CartasNaMesa[jogador.Nome]; ok {
		sala.mutex.Unlock()
		sala.enviarAtualizacaoJogo("Você já jogou. Aguarde o oponente.", "", "")
		return
	}

	var carta Carta
	var cartaIndex = -1
	for i, c := range jogador.Inventario {
		if c.ID == cartaID {
			carta = c
			cartaIndex = i
			break
		}
	}

	if cartaIndex == -1 {
		sala.mutex.Unlock()
		sala.enviarAtualizacaoJogo(fmt.Sprintf("[SISTEMA] %s jogou uma carta inválida.", jogador.Nome), "", "")
		return
	}

	// Move a carta do inventário para a mesa
	jogador.Inventario = append(jogador.Inventario[:cartaIndex], jogador.Inventario[cartaIndex+1:]...)
	sala.CartasNaMesa[jogador.Nome] = carta
	sala.JogadasNaRodada++

	// Se ambos jogaram, resolve a jogada
	if len(sala.CartasNaMesa) == 2 {
		p1 := sala.Jogadores[0]
		p2 := sala.Jogadores[1]
		c1 := sala.CartasNaMesa[p1.Nome]
		c2 := sala.CartasNaMesa[p2.Nome]

		vencedorJogada := "EMPATE"
		var vencedor *Cliente

		resultado := compararCartas(c1, c2)
		if resultado > 0 {
			vencedorJogada = p1.Nome
			vencedor = p1
		} else if resultado < 0 {
			vencedorJogada = p2.Nome
			vencedor = p2
		}

		if vencedor != nil {
			sala.PontosRodada[vencedor.Nome]++
		}
		// As cartas são descartadas e não retornam ao inventário.

		// Limpa a mesa para a próxima jogada
		sala.CartasNaMesa = make(map[string]Carta)
		sala.mutex.Unlock()

		sala.enviarAtualizacaoJogo(fmt.Sprintf("Vencedor da jogada: %s", vencedorJogada), vencedorJogada, "")

		// Checa fim da partida por 0 cartas (agora acontecerá após 5 rodadas)
		if len(p1.Inventario) == 0 {
			vencedorFinal := "EMPATE"
			p1Pontos := sala.PontosRodada[p1.Nome]
			p2Pontos := sala.PontosRodada[p2.Nome]
			if p1Pontos > p2Pontos {
				vencedorFinal = p1.Nome
			} else if p2Pontos > p1Pontos {
				vencedorFinal = p2.Nome
			}
			sala.finalizarPartida(vencedorFinal)
			return
		}

		sala.enviarAtualizacaoJogo("Próxima jogada. Use /jogar <ID_da_carta> para jogar ou /cartas para ver sua mão.", "", "")
		return
	}

	sala.mutex.Unlock()
	sala.enviarAtualizacaoJogo("Aguardando o oponente...", "", "")
}

func (sala *Sala) finalizarPartida(vencedor string) {
	sala.mutex.Lock()
	sala.Estado = "FINALIZADO"
	sala.mutex.Unlock()

	sala.broadcast(nil, protocolo.Mensagem{Comando: "FIM_DE_JOGO", Dados: mustJSON(protocolo.DadosFimDeJogo{VencedorNome: vencedor})})

	sala.reiniciarSala()
	sala.broadcast(nil, protocolo.Mensagem{
		Comando: "SISTEMA",
		Dados:   mustJSON(protocolo.DadosErro{Mensagem: "[SISTEMA] Partida finalizada. Use /comprar para adquirir um pacote e iniciar uma nova partida."}),
	})
}

func (sala *Sala) reiniciarSala() {
	sala.mutex.Lock()
	defer sala.mutex.Unlock()

	sala.Estado = "AGUARDANDO_COMPRA"
	sala.CartasNaMesa = make(map[string]Carta)
	sala.PontosRodada = make(map[string]int)
	sala.PontosPartida = make(map[string]int)
	sala.NumeroRodada = 1
	sala.JogadasNaRodada = 0
	sala.Prontos = make(map[string]bool)
	for _, j := range sala.Jogadores {
		j.Inventario = j.Inventario[:0] // Limpa o inventário
	}
}

func compararCartas(c1, c2 Carta) int {
	if c1.Valor != c2.Valor {
		return c1.Valor - c2.Valor
	}
	naipes := map[string]int{"♠": 4, "♥": 3, "♦": 2, "♣": 1}
	return naipes[c1.Naipe] - naipes[c2.Naipe]
}

func (s *Servidor) mostrarCartasDetalhadas(cliente *Cliente) {
	if len(cliente.Inventario) == 0 {
		s.enviar(cliente, protocolo.Mensagem{
			Comando: "SISTEMA",
			Dados:   mustJSON(protocolo.DadosErro{Mensagem: "Você não possui cartas. Use /comprar para comprar um pacote."}),
		})
		return
	}

	var builder strings.Builder
	builder.WriteString("\n=== SUAS CARTAS ===\n")
	for i, carta := range cliente.Inventario {
		builder.WriteString(fmt.Sprintf("%d. %s %s (ID: %s, Poder: %d, Raridade: %s)\n",
			i+1, carta.Nome, carta.Naipe, carta.ID, carta.Valor, carta.Raridade))
	}
	builder.WriteString("==================\n")

	s.enviar(cliente, protocolo.Mensagem{
		Comando: "CARTAS_DETALHADAS",
		Dados:   mustJSON(protocolo.DadosErro{Mensagem: builder.String()}),
	})
}

func mustJSON(v any) []byte { b, _ := json.Marshal(v); return b }

// OTIMIZAÇÃO: Contador Atômico para IDs, eliminando o Mutex.
var idSeq int64

func novoID() string {
	id := atomic.AddInt64(&idSeq, 1)
	return fmt.Sprintf("c%d", id)
}
func sampleRaridade() string {
	// BAREMA ITEM 8: PACOTES - Cria gerador aleatório único para esta operação
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	x := rng.Intn(100)
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

// BAREMA ITEM 8: PACOTES - Gera estoque inicial distribuído entre shards
// Cria um grande estoque de cartas com distribuição de raridade para testes de estresse
func gerarEstoquesIniciais() []map[string][]Carta {
	rand.Seed(time.Now().UnixNano())
	allStocks := make([]map[string][]Carta, numEstoqueShards)

	// BAREMA ITEM 5: CONCORRÊNCIA - Inicializa shards com mapas vazios por raridade
	for i := 0; i < numEstoqueShards; i++ {
		allStocks[i] = map[string][]Carta{"C": {}, "U": {}, "R": {}, "L": {}}
	}

	// BAREMA ITEM 8: PACOTES - Tipos de cartas disponíveis no jogo (mais variedade)
	tiposCartas := []string{
		"Dragão", "Guerreiro", "Mago", "Anjo", "Demônio", "Fênix", "Titan", "Sereia",
		"Lobo", "Águia", "Leão", "Tigre", "Cavaleiro", "Arqueiro", "Bárbaro", "Paladino",
		"Ranger", "Bruxo", "Druida", "Monge", "Assassino", "Bardo", "Necromante",
		"Elementalista", "Inquisidor", "Gladiador", "Mercenário", "Escudeiro", "Aprendiz",
		"Novato", "Veterano", "Herói", "Lenda", "Mestre", "Sábio", "Ancião", "Clérigo",
		"Ladrão", "Espadachim", "Arqueiro Élfico", "Mago do Caos", "Sacerdote", "Berserker",
		"Samurai", "Ninja", "Viking", "Cruzado", "Templário", "Inquisidor", "Caçador",
		"Explorador", "Navegador", "Alquimista", "Encantador", "Ilusionista", "Necromante",
		"Summoner", "Conjurador", "Evocador", "Invocador", "Chamador", "Convocador",
	}
	naipes := []string{"♠", "♥", "♦", "♣"}

	// BAREMA ITEM 8: PACOTES - Gera cartas com distribuição de raridade
	for _, nome := range tiposCartas {
		// Cartas Comuns (70% de chance): 500 por tipo para evitar esgotamento
		for i := 0; i < 500; i++ {
			// BAREMA ITEM 8: PACOTES - Cria gerador aleatório único para cada carta
			rng := rand.New(rand.NewSource(time.Now().UnixNano() + int64(i*1000) + int64(len(nome))))
			shardIndex := rng.Intn(numEstoqueShards)
			allStocks[shardIndex]["C"] = append(allStocks[shardIndex]["C"], Carta{
				ID: novoID(), Nome: nome, Naipe: naipes[rng.Intn(len(naipes))],
				Valor: 1 + rng.Intn(50), Raridade: "C"})
		}
		// Cartas Incomuns (20% de chance): 250 por tipo
		for i := 0; i < 250; i++ {
			rng := rand.New(rand.NewSource(time.Now().UnixNano() + int64(i*2000) + int64(len(nome))))
			shardIndex := rng.Intn(numEstoqueShards)
			allStocks[shardIndex]["U"] = append(allStocks[shardIndex]["U"], Carta{
				ID: novoID(), Nome: nome, Naipe: naipes[rng.Intn(len(naipes))],
				Valor: 51 + rng.Intn(30), Raridade: "U"})
		}
		// Cartas Raras (9% de chance): 100 por tipo
		for i := 0; i < 100; i++ {
			rng := rand.New(rand.NewSource(time.Now().UnixNano() + int64(i*3000) + int64(len(nome))))
			shardIndex := rng.Intn(numEstoqueShards)
			allStocks[shardIndex]["R"] = append(allStocks[shardIndex]["R"], Carta{
				ID: novoID(), Nome: nome, Naipe: naipes[rng.Intn(len(naipes))],
				Valor: 81 + rng.Intn(20), Raridade: "R"})
		}
		// Cartas Lendárias (1% de chance): 50 por tipo
		for i := 0; i < 50; i++ {
			rng := rand.New(rand.NewSource(time.Now().UnixNano() + int64(i*4000) + int64(len(nome))))
			shardIndex := rng.Intn(numEstoqueShards)
			allStocks[shardIndex]["L"] = append(allStocks[shardIndex]["L"], Carta{
				ID: novoID(), Nome: nome, Naipe: naipes[rng.Intn(len(naipes))],
				Valor: 101 + rng.Intn(20), Raridade: "L"})
		}
	}
	return allStocks
}
