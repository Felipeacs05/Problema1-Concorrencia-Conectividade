// felipeacs05/problema1-concorrencia-conectividade/Problema1-Concorrencia-Conectividade-77d73bcc575bbc2b6e076d0c153ffc2b7b175855/Projeto/servidor/main.go
package main

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"meujogo/protocolo"
	"net"
	"strings"
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
	UltimoPing time.Time // Adicionado para health check
	PingMs     int64     // Adicionado para health check
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
	ID              string
	Jogadores       []*Cliente
	Estado          string           // "AGUARDANDO_COMPRA" | "JOGANDO" | "FINALIZADO"
	CartasNaMesa    map[string]Carta // nome -> carta jogada na jogada atual
	PontosRodada    map[string]int   // nome -> pontos na rodada atual
	PontosPartida   map[string]int   // nome -> rodadas ganhas
	NumeroRodada    int              // 1, 2, 3
	JogadasNaRodada int              // 0, 1, 2 (quantas jogadas já foram feitas na rodada)
	Prontos         map[string]bool  // quem já comprou /buy_pack nesta partida
	srv             *Servidor
	mutex           sync.Mutex
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
	filaDeEspera   *Cliente // Ponteiro para o cliente que está aguardando
	filaMutex      sync.Mutex
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
		shardedEstoque: make([]*estoqueShard, numEstoqueShards),
	}
	// A inicialização do shardedFila é removida, pois não será mais usada
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
		// Envia mensagem de ajuda após a compra
		s.enviar(req.cli, protocolo.Mensagem{
			Comando: "SISTEMA",
			Dados:   mustJSON(protocolo.DadosErro{Mensagem: "[SISTEMA] Você recebeu novas cartas! Use /cartas para ver sua mão."}),
		})
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
	cliente.UltimoPing = time.Now() // Adicionado para health check

	s.adicionarCliente(cliente)
	go s.clienteWriter(cliente)
	go s.pingManager(cliente) // Inicia o gerenciador de PING
	s.clienteReader(cliente)

	// OTIMIZAÇÃO: Devolve o objeto Cliente para o pool
	conn.Close() // Garante que a conexão seja fechada
	s.removerCliente(cliente)

	// Limpa o estado do cliente para reutilização
	cliente.Inventario = cliente.Inventario[:0] // Limpa o slice
	cliente.Sala = nil
	cliente.Conn = nil
	cliente.Nome = ""
	cliente.PingMs = 0
	cliente.UltimoPing = time.Time{}

	clientePool.Put(cliente)
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
func (s *Servidor) entrarFila(cliente *Cliente) {
	s.filaMutex.Lock()
	// Verifica se já existe um jogador na fila de espera
	if s.filaDeEspera != nil {
		oponente := s.filaDeEspera
		s.filaDeEspera = nil // Limpa a fila
		s.filaMutex.Unlock()
		// Cria a sala com os dois jogadores
		fmt.Printf("[SERVIDOR] Jogador %s encontrado para %s. Criando sala...\n", cliente.Nome, oponente.Nome)
		s.criarSala(oponente, cliente)
	} else {
		// Nenhum jogador esperando, este cliente se torna o jogador na fila
		s.filaDeEspera = cliente
		s.filaMutex.Unlock()
		fmt.Printf("[SERVIDOR] Jogador %s entrou na fila e está aguardando um oponente.\n", cliente.Nome)
		s.enviar(cliente, protocolo.Mensagem{
			Comando: "SISTEMA",
			Dados:   mustJSON(protocolo.DadosErro{Mensagem: "[SISTEMA] Aguardando um oponente..."}),
		})
	}
}
func (s *Servidor) criarSala(j1, j2 *Cliente) {
	salaID := novoID() // Usando nosso gerador rápido de ID
	novaSala := &Sala{
		ID:              salaID,
		Jogadores:       []*Cliente{j1, j2},
		Estado:          "AGUARDANDO_COMPRA",
		CartasNaMesa:    make(map[string]Carta),
		PontosRodada:    make(map[string]int),
		PontosPartida:   make(map[string]int),
		NumeroRodada:    1,
		JogadasNaRodada: 0,
		Prontos:         make(map[string]bool),
		srv:             s,
	}
	s.salas.Store(salaID, novaSala)
	j1.Sala, j2.Sala = novaSala, novaSala

	// Notifica os jogadores sobre a partida encontrada
	d1 := protocolo.DadosPartidaEncontrada{SalaID: salaID, OponenteNome: j2.Nome}
	d2 := protocolo.DadosPartidaEncontrada{SalaID: salaID, OponenteNome: j1.Nome}
	s.enviar(j1, protocolo.Mensagem{Comando: "PARTIDA_ENCONTRADA", Dados: mustJSON(d1)})
	s.enviar(j2, protocolo.Mensagem{Comando: "PARTIDA_ENCONTRADA", Dados: mustJSON(d2)})

	// Informar que devem comprar pack para iniciar
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
