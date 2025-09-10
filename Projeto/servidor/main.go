package main

import (
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"meujogo/protocolo"
	"net"
	"os"
	"strings"
	"sync"
	"time"
)

/* ====================== Tipos principais ====================== */

type Carta = protocolo.Carta

type Cliente struct {
	Conn       net.Conn
	Nome       string
	Encoder    *json.Encoder
	Mailbox    chan protocolo.Mensagem
	Sala       *Sala
	Inventario []Carta // cartas do jogador na rodada atual
	CartasJSON string  // arquivo JSON com as cartas do jogador
	UltimoPing time.Time
	PingMs     int64
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

type Servidor struct {
	clientes     sync.Map // map[net.Conn]*Cliente - thread-safe
	salas        sync.Map // map[string]*Sala - thread-safe
	filaDeEspera []*Cliente
	filaMutex    sync.RWMutex

	// ==== Estoque global de pacotes (actor) ====
	packReqs     chan packReq
	estoque      map[string][]Carta // raridade -> cartas disponíveis (IDs únicos)
	packSize     int                // 5 cartas por /buy_pack
	estoqueMutex sync.RWMutex       // Mutex para estoque

	// Pool de workers para processar pacotes
	packWorkers    int
	packWorkerPool chan packReq
}

type packReq struct {
	cli        *Cliente
	quantidade int // pacotes; cada um com packSize cartas
}

/* ====================== Servidor / bootstrap ====================== */

func novoServidor() *Servidor {
	s := &Servidor{
		packReqs:       make(chan packReq, 50000), // Buffer muito maior para 10k clientes
		estoque:        gerarEstoqueInicial(),     // C/U/R/L com IDs únicos
		packSize:       5,
		packWorkers:    200, // 200 workers para processar pacotes
		packWorkerPool: make(chan packReq, 50000),
	}

	// Inicia pool de workers para processar pacotes
	for i := 0; i < s.packWorkers; i++ {
		go s.packWorker()
	}

	return s
}

func main() {
	servidor := novoServidor()

	// Configurações para alta concorrência
	listener, err := net.Listen("tcp", ":65432")
	if err != nil {
		panic(err)
	}
	defer listener.Close()

	// Configurações de TCP para alta concorrência
	if tcpListener, ok := listener.(*net.TCPListener); ok {
		tcpListener.SetDeadline(time.Now().Add(1 * time.Hour))
	}

	fmt.Println("[SERVIDOR] ouvindo :65432 (otimizado para alta concorrência)")

	// Semáforo para limitar conexões simultâneas
	semaphore := make(chan struct{}, 5000) // Máximo 5000 conexões simultâneas para 10k clientes

	for {
		conn, err := listener.Accept()
		if err != nil {
			fmt.Printf("[SERVIDOR] Erro ao aceitar conexão: %v\n", err)
			continue
		}

		// Configurações de TCP para a conexão
		if tcpConn, ok := conn.(*net.TCPConn); ok {
			tcpConn.SetKeepAlive(true)
			tcpConn.SetKeepAlivePeriod(30 * time.Second)
			tcpConn.SetNoDelay(true) // Desabilita Nagle's algorithm
		}

		// Usa semáforo para controlar concorrência
		semaphore <- struct{}{}
		go func() {
			defer func() { <-semaphore }()
			servidor.handleConnection(conn)
		}()
	}
}

/* ====================== Pool de Workers para Pacotes ====================== */

// Worker individual para processar pacotes
func (s *Servidor) packWorker() {
	for req := range s.packWorkerPool {
		s.processarPacote(req)
	}
}

// Processa um pedido de pacote individual
func (s *Servidor) processarPacote(req packReq) {
	// Verifica se o cliente ainda está conectado
	if req.cli.Conn == nil {
		return
	}

	if req.quantidade <= 0 {
		req.quantidade = 1
	}
	totalNecessario := req.quantidade * s.packSize

	// Selecione cartas segundo distribuição de raridade com downgrade
	cartas := make([]Carta, 0, totalNecessario)
	for i := 0; i < totalNecessario; i++ {
		rar := sampleRaridade() // C=70 U=20 R=9 L=1
		c, ok := s.takeOneByRarityWithDowngrade(rar)
		if !ok {
			// estoque insuficiente - gera carta básica
			c = s.gerarCartaComumBasica()
		}
		cartas = append(cartas, c)
	}

	// Carrega cartas existentes do JSON
	cartasExistentes := carregarCartasJSON(req.cli)
	// Adiciona novas cartas
	cartasExistentes = append(cartasExistentes, cartas...)
	// Salva todas as cartas no JSON
	salvarCartasJSON(req.cli, cartasExistentes)
	// Atualiza inventário em memória
	req.cli.Inventario = cartasExistentes

	// Computa estoque restante em pacotes (cada pacote = 5 cartas)
	s.estoqueMutex.RLock()
	totalCartas := 0
	for _, v := range s.estoque {
		totalCartas += len(v)
	}
	rest := totalCartas / s.packSize
	s.estoqueMutex.RUnlock()

	// Envia resultado da compra
	msg := protocolo.Mensagem{
		Comando: "PACOTE_RESULTADO",
		Dados:   mustJSON(protocolo.ComprarPacoteResp{Cartas: cartas, EstoqueRestante: rest}),
	}

	sucesso := s.enviar(req.cli, msg)

	if sucesso {
		// Mostra cartas detalhadas após compra e marca como pronto
		mostrarCartasDetalhadas(req.cli)
		if req.cli.Sala != nil {
			req.cli.Sala.marcarCompraEIniciarSePossivel(req.cli)
		}
		fmt.Printf("[SERVIDOR] %s - pacote processado com sucesso\n", req.cli.Nome)
	}
}

func (s *Servidor) takeOneByRarityWithDowngrade(r string) (Carta, bool) {
	// Proteção contra concorrência no estoque
	s.estoqueMutex.Lock()
	defer s.estoqueMutex.Unlock()

	order := []string{"L", "R", "U", "C"} // tentar da pedida e ir "descendo"
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
		if arr := s.estoque[order[i]]; len(arr) > 0 {
			// pop do final (O(1))
			n := len(arr) - 1
			c := arr[n]
			s.estoque[order[i]] = arr[:n]
			return c, true
		}
	}

	// Se chegou aqui, estoque acabou - retorna false para que seja gerada carta básica
	return Carta{}, false
}

// Gera cartas comuns básicas quando o estoque acaba
func (s *Servidor) gerarCartaComumBasica() Carta {
	tiposBasicos := []string{
		"Soldado Básico", "Arqueiro Novato", "Mago Aprendiz", "Curandeiro Simples",
	}
	naipes := []string{"♠", "♥", "♦", "♣"}

	nome := tiposBasicos[rand.Intn(len(tiposBasicos))]
	naipe := naipes[rand.Intn(len(naipes))]
	poder := 5 + rand.Intn(15) // Poder 5-19 (mais fraco que cartas normais)

	return Carta{
		ID:       novoID(),
		Nome:     nome,
		Naipe:    naipe,
		Valor:    poder,
		Raridade: "C",
	}
}

/* ====================== Conexão / IO ====================== */

func (s *Servidor) handleConnection(conn net.Conn) {
	defer conn.Close()
	cliente := &Cliente{
		Conn:       conn,
		Nome:       conn.RemoteAddr().String(),
		Encoder:    json.NewEncoder(conn),
		Mailbox:    make(chan protocolo.Mensagem, 32),
		Inventario: make([]Carta, 0, 64),
		UltimoPing: time.Now(),
		PingMs:     0,
	}
	s.adicionarCliente(cliente)
	defer s.removerCliente(cliente)

	fmt.Printf("[SERVIDOR] nova conexão %s\n", cliente.Nome)
	go s.clienteWriter(cliente)
	go s.pingManager(cliente)
	s.clienteReader(cliente)
}

// Gerencia pings periódicos para o cliente
func (s *Servidor) pingManager(cliente *Cliente) {
	ticker := time.NewTicker(5 * time.Second) // Ping a cada 5 segundos
	defer ticker.Stop()

	for range ticker.C {
		// Verifica se a conexão ainda está ativa
		if cliente.Conn == nil {
			return
		}
		timestamp := time.Now().UnixMilli()
		// Tenta enviar ping, se falhar, encerra a goroutine
		if !s.enviar(cliente, protocolo.Mensagem{
			Comando: "PING",
			Dados:   mustJSON(protocolo.DadosPing{Timestamp: timestamp}),
		}) {
			return
		}
	}
}

func (s *Servidor) clienteWriter(c *Cliente) {
	for msg := range c.Mailbox {
		// Timeout para escrita
		c.Conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
		if err := c.Encoder.Encode(msg); err != nil {
			fmt.Printf("[SERVIDOR] erro de escrita para %s: %v\n", c.Nome, err)
			return
		}
	}
}

func (s *Servidor) clienteReader(cliente *Cliente) {
	decoder := json.NewDecoder(cliente.Conn)
	for {
		// Timeout para leitura
		cliente.Conn.SetReadDeadline(time.Now().Add(30 * time.Second))
		var msg protocolo.Mensagem
		if err := decoder.Decode(&msg); err != nil {
			if err == io.EOF {
				fmt.Printf("[SERVIDOR] %s desconectou (EOF)\n", cliente.Nome)
				return
			}
			fmt.Printf("[SERVIDOR] erro de leitura de %s: %v\n", cliente.Nome, err)
			return
		}

		switch msg.Comando {
		case "LOGIN":
			var dadosLogin protocolo.DadosLogin
			if err := json.Unmarshal(msg.Dados, &dadosLogin); err == nil && dadosLogin.Nome != "" {
				cliente.Nome = dadosLogin.Nome
				cliente.CartasJSON = fmt.Sprintf("cartas_%s.json", cliente.Nome)
				fmt.Printf("[SERVIDOR] %s fez login como '%s'\n", cliente.Conn.RemoteAddr().String(), cliente.Nome)
			}

		case "ENTRAR_NA_FILA":
			s.entrarFila(cliente)

		case "PRONTO":
			// Ignorado no novo fluxo; mantido por compatibilidade

		case "BUY_CARTA":
			// Mostra cartas detalhadas
			mostrarCartasDetalhadas(cliente)

		case "JOGAR_CARTA":
			if cliente.Sala == nil {
				break
			}
			if cliente.Sala.Estado == "JOGANDO" {
				// durante a partida, /jogar com seleção de carta
				var dadosJogar protocolo.DadosJogarCarta
				json.Unmarshal(msg.Dados, &dadosJogar)
				cliente.Sala.processarJogada(cliente, dadosJogar.CartaID)
			}
			if cliente.Sala.Estado == "AGUARDANDO_COMPRA" {
				cliente.Sala.enviarAtualizacaoJogo("Ambos devem comprar com /buy_pack antes de jogar.", "", "")
			}

		case "COMPRAR_PACOTE":
			fmt.Printf("[SERVIDOR] %s solicitou compra de pacote\n", cliente.Nome)

			// Permitido somente fora da partida (não durante o jogo)
			if cliente.Sala != nil && cliente.Sala.Estado == "JOGANDO" {
				s.enviar(cliente, protocolo.Mensagem{
					Comando: "ERRO",
					Dados:   mustJSON(protocolo.DadosErro{Mensagem: "Você só pode comprar cartas fora da partida."}),
				})
				break
			}
			var req protocolo.ComprarPacoteReq
			_ = json.Unmarshal(msg.Dados, &req)
			if req.Quantidade <= 0 {
				req.Quantidade = 1
			}

			// Envia para o pool de workers
			select {
			case s.packWorkerPool <- packReq{cli: cliente, quantidade: req.Quantidade}:
				// Sucesso
				fmt.Printf("[SERVIDOR] %s - pedido de pacote enviado para processamento\n", cliente.Nome)
			default:
				// Pool cheio, envia erro
				s.enviar(cliente, protocolo.Mensagem{
					Comando: "ERRO",
					Dados:   mustJSON(protocolo.DadosErro{Mensagem: "Servidor sobrecarregado. Tente novamente."}),
				})
			}

		case "ENVIAR_CHAT":
			if cliente.Sala != nil {
				var dadosChat protocolo.DadosEnviarChat
				if err := json.Unmarshal(msg.Dados, &dadosChat); err == nil {
					dados := protocolo.DadosReceberChat{NomeJogador: cliente.Nome, Texto: dadosChat.Texto}
					jsonDados, _ := json.Marshal(dados)
					msgParaBroadcast := protocolo.Mensagem{Comando: "RECEBER_CHAT", Dados: jsonDados}
					cliente.Sala.broadcast(nil, msgParaBroadcast)
				}
			}

		case "PING":
			var dadosPing protocolo.DadosPing
			if err := json.Unmarshal(msg.Dados, &dadosPing); err == nil {
				// Responde com PONG
				pong := protocolo.DadosPong(dadosPing)
				s.enviar(cliente, protocolo.Mensagem{
					Comando: "PONG",
					Dados:   mustJSON(pong),
				})
			}

		case "PONG":
			var dadosPong protocolo.DadosPong
			if err := json.Unmarshal(msg.Dados, &dadosPong); err == nil {
				// Calcula ping
				agora := time.Now().UnixMilli()
				cliente.PingMs = agora - dadosPong.Timestamp
				cliente.UltimoPing = time.Now()
				fmt.Printf("[SERVIDOR] Ping de %s: %dms\n", cliente.Nome, cliente.PingMs)
			}
		}
	}
}

/* ====================== Matchmaking ====================== */

func (s *Servidor) entrarFila(cliente *Cliente) {
	s.filaMutex.Lock()
	defer s.filaMutex.Unlock()
	s.filaDeEspera = append(s.filaDeEspera, cliente)
	if len(s.filaDeEspera) >= 2 {
		s.criarSalaComJogadoresDaFila()
	}
}

func (s *Servidor) criarSalaComJogadoresDaFila() {
	j1 := s.filaDeEspera[0]
	j2 := s.filaDeEspera[1]
	s.filaDeEspera = s.filaDeEspera[2:]

	salaID := fmt.Sprintf("sala-%d", time.Now().UnixNano())
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
	// Carrega cartas existentes do JSON para cada jogador
	j1.Inventario = carregarCartasJSON(j1)
	j2.Inventario = carregarCartasJSON(j2)
	s.salas.Store(salaID, novaSala)
	j1.Sala, j2.Sala = novaSala, novaSala

	// Notifica os dois sobre a sala criada
	d1 := protocolo.DadosPartidaEncontrada{SalaID: salaID, OponenteNome: j2.Nome}
	d2 := protocolo.DadosPartidaEncontrada{SalaID: salaID, OponenteNome: j1.Nome}
	s.enviar(j1, protocolo.Mensagem{Comando: "PARTIDA_ENCONTRADA", Dados: mustJSON(d1)})
	s.enviar(j2, protocolo.Mensagem{Comando: "PARTIDA_ENCONTRADA", Dados: mustJSON(d2)})

	// Informar que devem comprar pack para iniciar
	novaSala.broadcast(nil, protocolo.Mensagem{
		Comando: "SISTEMA",
		Dados:   mustJSON(protocolo.DadosErro{Mensagem: "[SISTEMA] Ambos jogadores devem digitar /buy_pack. Quando ambos comprarem, a partida inicia."}),
	})
}

/* ====================== Sala: lobby e jogo ====================== */

// marcarCompraEIniciarSePossivel marca o jogador que comprou e verifica se pode iniciar a partida
func (sala *Sala) marcarCompraEIniciarSePossivel(cli *Cliente) {
	sala.mutex.Lock()

	// Se estava finalizada, reinicia tudo
	if sala.Estado == "FINALIZADO" {
		sala.reiniciarSala()
	}
	if sala.Prontos == nil {
		sala.Prontos = make(map[string]bool)
	}

	// Marca como "comprou"
	sala.Prontos[cli.Nome] = true
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

/* ====================== Sala: métodos de broadcast e jogo ====================== */

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

// removerJogador remove um jogador da sala
func (sala *Sala) removerJogador(cliente *Cliente) {
	sala.mutex.Lock()
	defer sala.mutex.Unlock()

	// Remove o jogador da lista
	for i, j := range sala.Jogadores {
		if j == cliente {
			sala.Jogadores = append(sala.Jogadores[:i], sala.Jogadores[i+1:]...)
			break
		}
	}

	// Notifica o oponente se houver
	if len(sala.Jogadores) > 0 {
		sala.broadcast(nil, protocolo.Mensagem{
			Comando: "SISTEMA",
			Dados:   mustJSON(protocolo.DadosErro{Mensagem: fmt.Sprintf("[SISTEMA] %s desconectou da partida", cliente.Nome)}),
		})
	}

	fmt.Printf("[SALA %s] Jogador %s removido\n", sala.ID, cliente.Nome)
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

/* ====================== Jogo ====================== */

// iniciarPartida inicia a partida
func (sala *Sala) iniciarPartida() {
	sala.mutex.Lock()
	sala.Estado = "JOGANDO"
	sala.CartasNaMesa = make(map[string]Carta)
	sala.JogadasNaRodada = 0
	sala.Prontos = make(map[string]bool)
	// Inicializa pontos se necessário
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

	sala.enviarAtualizacaoJogo("[SISTEMA] PARTIDA INICIADA, cada jogador deve digitar /jogar <cartaID> para fazer a jogada", "", "")
}

func (sala *Sala) criarAtualizacaoJogo(mensagem, vencedorJogada, vencedorRodada string) protocolo.DadosAtualizacaoJogo {
	contagem := make(map[string]int)
	ultima := make(map[string]Carta)
	for _, p := range sala.Jogadores {
		contagem[p.Nome] = len(p.Inventario)
		if c, ok := sala.CartasNaMesa[p.Nome]; ok {
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

// Cria atualização personalizada escondendo poder das cartas do oponente
func (sala *Sala) criarAtualizacaoJogoPersonalizada(mensagem, vencedorJogada, vencedorRodada string, jogadorAtual *Cliente) protocolo.DadosAtualizacaoJogo {
	contagem := make(map[string]int)
	ultima := make(map[string]Carta)

	for _, p := range sala.Jogadores {
		contagem[p.Nome] = len(p.Inventario)
		if c, ok := sala.CartasNaMesa[p.Nome]; ok {
			if p.Nome == jogadorAtual.Nome {
				// Mostra carta completa para o próprio jogador
				ultima[p.Nome] = c
			} else {
				// Esconde o poder da carta do oponente
				cartaOculta := c
				cartaOculta.Valor = 0 // Esconde o poder
				ultima[p.Nome] = cartaOculta
			}
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

func (sala *Sala) processarJogada(jogador *Cliente, cartaID string) {
	sala.mutex.Lock()

	if sala.Estado != "JOGANDO" {
		sala.mutex.Unlock()
		return
	}

	// já jogou nesta jogada?
	if _, ok := sala.CartasNaMesa[jogador.Nome]; ok {
		sala.mutex.Unlock()
		sala.enviarAtualizacaoJogo("Você já jogou nesta jogada. Aguarde o oponente.", "", "")
		return
	}

	// Carrega cartas atuais do JSON
	jogador.Inventario = carregarCartasJSON(jogador)

	// Encontra a carta no inventário do jogador (por ID)
	var carta Carta
	var cartaIndex = -1
	entrada := strings.TrimSpace(cartaID)

	for i, c := range jogador.Inventario {
		if c.ID == entrada {
			carta = c
			cartaIndex = i
			break
		}
	}

	if cartaIndex == -1 {
		// Carta inválida - notifica o oponente
		sala.mutex.Unlock()
		sala.enviarAtualizacaoJogo(fmt.Sprintf("[SISTEMA] - Jogador %s jogou uma carta inválida, esperando para jogar carta correta", jogador.Nome), "", "")
		return
	}

	// Remove a carta do inventário e coloca na mesa
	jogador.Inventario = append(jogador.Inventario[:cartaIndex], jogador.Inventario[cartaIndex+1:]...)
	salvarCartasJSON(jogador, jogador.Inventario) // salva a remoção
	sala.CartasNaMesa[jogador.Nome] = carta
	sala.JogadasNaRodada++

	// se os dois já jogaram, resolve a jogada
	if len(sala.CartasNaMesa) == 2 {
		p1 := sala.Jogadores[0]
		p2 := sala.Jogadores[1]
		c1 := sala.CartasNaMesa[p1.Nome]
		c2 := sala.CartasNaMesa[p2.Nome]

		vencedorJogada := "EMPATE"
		var vencedor *Cliente
		var cartaPerdedora Carta

		resultado := compararCartas(c1, c2)
		if resultado > 0 {
			vencedorJogada = p1.Nome
			sala.PontosRodada[p1.Nome]++
			vencedor = p1
			cartaPerdedora = c2
		} else if resultado < 0 {
			vencedorJogada = p2.Nome
			sala.PontosRodada[p2.Nome]++
			vencedor = p2
			cartaPerdedora = c1
		} else {
			// Empate: tie-breaker determinístico por nome (ou ID de conexão)
			if p1.Nome < p2.Nome {
				vencedorJogada = p1.Nome
				sala.PontosRodada[p1.Nome]++
				vencedor = p1
				cartaPerdedora = c2
			} else if p2.Nome < p1.Nome {
				vencedorJogada = p2.Nome
				sala.PontosRodada[p2.Nome]++
				vencedor = p2
				cartaPerdedora = c1
			} else {
				// nomes iguais são extremamente improváveis; manter empate real
				vencedorJogada = "EMPATE"
			}
		}

		// Transferência de cartas se houver vencedor
		if vencedorJogada != "EMPATE" {
			// Vencedor ganha a carta do perdedor
			vencedor.Inventario = append(vencedor.Inventario, cartaPerdedora)
			salvarCartasJSON(vencedor, vencedor.Inventario)
		}

		// Mostra resultado da jogada
		sala.mutex.Unlock()
		sala.enviarAtualizacaoJogo(fmt.Sprintf("Vencedor da jogada: %s", vencedorJogada), vencedorJogada, "")

		// Mostra cartas atualizadas para ambos jogadores
		mostrarCartasDetalhadas(p1)
		mostrarCartasDetalhadas(p2)

		// Limpa mesa para próxima jogada
		sala.mutex.Lock()
		sala.CartasNaMesa = make(map[string]Carta)
		sala.mutex.Unlock()
		sala.enviarAtualizacaoJogo("Próxima jogada - Use /jogar <cartaID>", "", "")

		// Checa fim da partida por 0 cartas
		if len(p1.Inventario) == 0 || len(p2.Inventario) == 0 {
			vencedorFinal := p1.Nome
			if len(p1.Inventario) == 0 && len(p2.Inventario) > 0 {
				vencedorFinal = p2.Nome
			}
			sala.finalizarPartidaPorZeroCartas(vencedorFinal)
			return
		}
		return
	}

	// apenas um jogou: avisa e mostra carta na mesa
	sala.mutex.Unlock()
	sala.enviarAtualizacaoJogo("Aguardando o oponente...", "", "")
}

func (sala *Sala) finalizarRodada() {
	sala.mutex.Lock()
	defer sala.mutex.Unlock()

	// Determina vencedor da rodada
	vencedorRodada := "EMPATE"
	if sala.PontosRodada[sala.Jogadores[0].Nome] > sala.PontosRodada[sala.Jogadores[1].Nome] {
		vencedorRodada = sala.Jogadores[0].Nome
		sala.PontosPartida[sala.Jogadores[0].Nome]++
	} else if sala.PontosRodada[sala.Jogadores[1].Nome] > sala.PontosRodada[sala.Jogadores[0].Nome] {
		vencedorRodada = sala.Jogadores[1].Nome
		sala.PontosPartida[sala.Jogadores[1].Nome]++
	}

	// Mostra vencedor da rodada
	sala.enviarAtualizacaoJogo(fmt.Sprintf("Vencedor da rodada %d: %s", sala.NumeroRodada, vencedorRodada), "", vencedorRodada)

	// Limpa inventários dos jogadores e arquivos JSON
	for _, p := range sala.Jogadores {
		limparCartasJSON(p)
	}

	// Em novo fluxo, rodadas não importam; apenas limpa e aguarda próxima jogada
	sala.JogadasNaRodada = 0
	sala.CartasNaMesa = make(map[string]Carta)
	sala.PontosRodada = make(map[string]int)
	for _, p := range sala.Jogadores {
		sala.PontosRodada[p.Nome] = 0
	}
	sala.mutex.Unlock()
	sala.enviarAtualizacaoJogo("Nova jogada - Use /jogar <cartaID>", "", "")
}

// finalizarPartidaPorZeroCartas finaliza a partida e reseta sala e estoque
func (sala *Sala) finalizarPartidaPorZeroCartas(vencedor string) {
	sala.mutex.Lock()
	sala.Estado = "FINALIZADO"
	// limpar JSONs e inventários
	for _, p := range sala.Jogadores {
		limparCartasJSON(p)
	}
	// restaurar estoque no servidor
	if sala.srv != nil {
		sala.srv.estoque = gerarEstoqueInicial()
	}
	sala.mutex.Unlock()

	sala.broadcast(nil, protocolo.Mensagem{Comando: "FIM_DE_JOGO", Dados: mustJSON(protocolo.DadosFimDeJogo{VencedorNome: vencedor})})

	// Reiniciar sala para nova partida
	sala.reiniciarSala()
	sala.broadcast(nil, protocolo.Mensagem{
		Comando: "SISTEMA",
		Dados:   mustJSON(protocolo.DadosErro{Mensagem: "[SISTEMA] Partida finalizada. Use /buy_pack para iniciar uma nova."}),
	})
}

/* ====================== Utilidades ====================== */

func (s *Servidor) adicionarCliente(c *Cliente) {
	s.clientes.Store(c.Conn, c)
}

func (s *Servidor) removerCliente(c *Cliente) {
	// Remove da fila de espera se estiver lá
	s.filaMutex.Lock()
	for i, cliente := range s.filaDeEspera {
		if cliente == c {
			s.filaDeEspera = append(s.filaDeEspera[:i], s.filaDeEspera[i+1:]...)
			break
		}
	}
	s.filaMutex.Unlock()

	// Remove da sala se estiver em uma
	if c.Sala != nil {
		c.Sala.removerJogador(c)
	}

	s.clientes.Delete(c.Conn)
	close(c.Mailbox)
}

func (s *Servidor) enviar(cli *Cliente, msg protocolo.Mensagem) bool {
	// Verifica se o cliente ainda está conectado
	if cli.Conn == nil {
		return false
	}

	select {
	case cli.Mailbox <- msg:
		return true
	case <-time.After(1 * time.Second):
		fmt.Printf("[SERVIDOR] timeout enviando para %s\n", cli.Nome)
		return false
	default:
		fmt.Printf("[SERVIDOR] mailbox cheia de %s, descartando\n", cli.Nome)
		return false
	}
}

func mustJSON(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}

/* ===== Funções auxiliares da sala ===== */

// reiniciarSala limpa todos os dados da sala para uma nova partida
func (sala *Sala) reiniciarSala() {
	sala.Estado = "AGUARDANDO_COMPRA"
	sala.CartasNaMesa = make(map[string]Carta)
	sala.PontosRodada = make(map[string]int)
	sala.PontosPartida = make(map[string]int)
	sala.NumeroRodada = 1
	sala.JogadasNaRodada = 0
	sala.Prontos = make(map[string]bool)
	// Limpa arquivos JSON das cartas
	for _, j := range sala.Jogadores {
		limparCartasJSON(j)
	}
}

/* ===== Funções para manipulação de arquivos JSON ===== */

// carregarCartasJSON carrega as cartas do arquivo JSON do jogador
func carregarCartasJSON(cliente *Cliente) []Carta {
	data, err := os.ReadFile(cliente.CartasJSON)
	if err != nil {
		return []Carta{} // se não existe, retorna vazio
	}

	var cartas []Carta
	if err := json.Unmarshal(data, &cartas); err != nil {
		return []Carta{}
	}
	return cartas
}

// salvarCartasJSON salva as cartas no arquivo JSON do jogador
func salvarCartasJSON(cliente *Cliente, cartas []Carta) {
	data, err := json.MarshalIndent(cartas, "", "  ")
	if err != nil {
		return
	}
	os.WriteFile(cliente.CartasJSON, data, 0644)
}

// limparCartasJSON remove o arquivo JSON do jogador
func limparCartasJSON(cliente *Cliente) {
	os.Remove(cliente.CartasJSON)
	cliente.Inventario = []Carta{}
}

// mostrarCartasDetalhadas envia todas as cartas detalhadas para o jogador
func mostrarCartasDetalhadas(cliente *Cliente) {
	cartas := carregarCartasJSON(cliente)
	cliente.Inventario = cartas

	if len(cartas) == 0 {
		cliente.Mailbox <- protocolo.Mensagem{
			Comando: "CARTAS_DETALHADAS",
			Dados:   mustJSON(protocolo.DadosErro{Mensagem: "Você não possui cartas. Use /buy_pack para comprar um pacote."}),
		}
		return
	}

	mensagem := "\n=== SUAS CARTAS ===\n"
	for i, carta := range cartas {
		mensagem += fmt.Sprintf("%d. %s %s (ID: %s, Poder: %d, Raridade: %s)\n",
			i+1, carta.Nome, carta.Naipe, carta.ID, carta.Valor, carta.Raridade)
	}
	mensagem += "\nUse: /jogar <cartaID> para jogar uma carta\n"
	mensagem += "==================\n"

	cliente.Mailbox <- protocolo.Mensagem{
		Comando: "CARTAS_DETALHADAS",
		Dados:   mustJSON(protocolo.DadosErro{Mensagem: mensagem}),
	}
}

/* ===== Comparação de cartas ===== */

// compararCartas retorna:
// > 0 se c1 vence c2
// < 0 se c2 vence c1
// = 0 se empate
func compararCartas(c1, c2 Carta) int {
	// Primeiro compara por valor
	if c1.Valor != c2.Valor {
		return c1.Valor - c2.Valor
	}

	// Se valores iguais, compara por naipe (hierarquia: ♠ > ♥ > ♦ > ♣)
	naipes := map[string]int{"♠": 4, "♥": 3, "♦": 2, "♣": 1}
	valor1 := naipes[c1.Naipe]
	valor2 := naipes[c2.Naipe]

	return valor1 - valor2
}

/* ===== Estoque inicial e baralho de partida ===== */

var idSeq int64
var idMu sync.Mutex

func novoID() string {
	idMu.Lock()
	defer idMu.Unlock()
	idSeq++
	return fmt.Sprintf("c%06d", idSeq)
}

func gerarEstoqueInicial() map[string][]Carta {
	rand.Seed(time.Now().UnixNano())

	// 12 tipos de cartas com múltiplas instâncias cada
	tiposCartas := []string{
		"Dragão de Fogo", "Guerreiro Lendário", "Mago Supremo", "Anjo Guardião",
		"Demônio das Trevas", "Fênix Renascida", "Titan de Pedra", "Sereia Encantadora",
		"Lobo Selvagem", "Águia Real", "Leão Majestoso", "Tigre Feroz",
	}

	naipes := []string{"♠", "♥", "♦", "♣"}
	out := map[string][]Carta{"C": {}, "U": {}, "R": {}, "L": {}}

	// Para cada tipo de carta, criamos múltiplas instâncias com raridades diferentes
	for _, nome := range tiposCartas {
		// Comuns: 5 instâncias de cada tipo
		for i := 0; i < 5; i++ {
			poder := 10 + rand.Intn(30) // Poder 10-39
			naipe := naipes[rand.Intn(len(naipes))]
			out["C"] = append(out["C"], Carta{
				ID:       novoID(),
				Nome:     nome,
				Naipe:    naipe,
				Valor:    poder,
				Raridade: "C",
			})
		}

		// Incomuns: 3 instâncias de cada tipo
		for i := 0; i < 3; i++ {
			poder := 40 + rand.Intn(30) // Poder 40-69
			naipe := naipes[rand.Intn(len(naipes))]
			out["U"] = append(out["U"], Carta{
				ID:       novoID(),
				Nome:     nome,
				Naipe:    naipe,
				Valor:    poder,
				Raridade: "U",
			})
		}

		// Raras: 2 instâncias de cada tipo (estoque limitado)
		for i := 0; i < 2; i++ {
			poder := 70 + rand.Intn(20) // Poder 70-89
			naipe := naipes[rand.Intn(len(naipes))]
			out["R"] = append(out["R"], Carta{
				ID:       novoID(),
				Nome:     nome,
				Naipe:    naipe,
				Valor:    poder,
				Raridade: "R",
			})
		}

		// Lendárias: 1 instância de cada tipo (muito escasso)
		poder := 90 + rand.Intn(10) // Poder 90-99
		naipe := naipes[rand.Intn(len(naipes))]
		out["L"] = append(out["L"], Carta{
			ID:       novoID(),
			Nome:     nome,
			Naipe:    naipe,
			Valor:    poder,
			Raridade: "L",
		})
	}

	fmt.Printf("[SERVIDOR] Estoque inicial gerado: C=%d, U=%d, R=%d, L=%d\n",
		len(out["C"]), len(out["U"]), len(out["R"]), len(out["L"]))
	return out
}

func sampleRaridade() string {
	// C=70, U=20, R=9, L=1 (100)
	x := rand.Intn(100)
	switch {
	case x < 70:
		return "C"
	case x < 90:
		return "U"
	case x < 99:
		return "R"
	default:
		return "L"
	}
}
