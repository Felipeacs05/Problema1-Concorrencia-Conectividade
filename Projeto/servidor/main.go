package main

import (
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"meujogo/protocolo"
	"net"
	"sync"
	"time"
)

/* ====================== Tipos principais ====================== */

type Carta = protocolo.Carta

type Cliente struct {
	Conn      net.Conn
	Nome      string
	Encoder   *json.Encoder
	Mailbox   chan protocolo.Mensagem
	Sala      *Sala
	Inventario []Carta // coleção do jogador (cresce com /buy_pack)
}

type Sala struct {
	ID           string
	Jogadores    []*Cliente
	Estado       string // "LOBBY" | "JOGANDO" | "FINALIZADO"
	Baralhos     map[string][]Carta     // nome -> deck da partida (10)
	CartasNaMesa map[string]Carta       // nome -> carta jogada na rodada
	Pontos       map[string]int         // placar
	Prontos      map[string]bool        // lobby: quem já pediu /jogar
	mutex        sync.Mutex
}

type Servidor struct {
	clientes     map[net.Conn]*Cliente
	salas        map[string]*Sala
	filaDeEspera []*Cliente
	mutex        sync.Mutex

	// ==== Estoque global de pacotes (actor) ====
	packReqs chan packReq
	estoque  map[string][]Carta // raridade -> cartas disponíveis (IDs únicos)
	packSize int                 // 10 cartas por /buy_pack
}

type packReq struct {
	cli        *Cliente
	quantidade int // pacotes; cada um com packSize cartas
}

/* ====================== Servidor / bootstrap ====================== */

func novoServidor() *Servidor {
	s := &Servidor{
		clientes: make(map[net.Conn]*Cliente),
		salas:    make(map[string]*Sala),

		packReqs: make(chan packReq, 128),
		estoque:  gerarEstoqueInicial(), // C/U/R/L com IDs únicos
		packSize: 10,
	}
	go s.loopPacotes() // actor do estoque global
	return s
}

func main() {
	servidor := novoServidor()
	listener, err := net.Listen("tcp", ":65432")
	if err != nil {
		panic(err)
	}
	defer listener.Close()
	fmt.Println("[SERVIDOR] ouvindo :65432")

	for {
		conn, _ := listener.Accept()
		go servidor.handleConnection(conn)
	}
}

/* ====================== Actor de Pacotes ====================== */

func (s *Servidor) loopPacotes() {
	for req := range s.packReqs {
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
				// estoque insuficiente
				s.enviar(req.cli, protocolo.Mensagem{
					Comando: "ERRO",
					Dados:   mustJSON(protocolo.DadosErro{Mensagem: "Estoque insuficiente para completar o pacote."}),
				})
				// devolve o que já tirou (para simplicidade, como não “aplicamos” ainda, não precisa devolver)
				cartas = nil
				break
			}
			cartas = append(cartas, c)
		}
		if len(cartas) == 0 {
			continue
		}

		// Entrega atômica para o jogador
		req.cli.Inventario = append(req.cli.Inventario, cartas...)

		// Computa estoque restante
		rest := 0
		for _, v := range s.estoque {
			rest += len(v)
		}

		s.enviar(req.cli, protocolo.Mensagem{
			Comando: "PACOTE_RESULTADO",
			Dados:   mustJSON(protocolo.ComprarPacoteResp{Cartas: cartas, EstoqueRestante: rest}),
		})
	}
}

func (s *Servidor) takeOneByRarityWithDowngrade(r string) (Carta, bool) {
	order := []string{"L", "R", "U", "C"} // tentar da pedida e ir “descendo”
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
	return Carta{}, false
}

/* ====================== Conexão / IO ====================== */

func (s *Servidor) handleConnection(conn net.Conn) {
	defer conn.Close()
	cliente := &Cliente{
		Conn:      conn,
		Nome:      conn.RemoteAddr().String(),
		Encoder:   json.NewEncoder(conn),
		Mailbox:   make(chan protocolo.Mensagem, 32),
		Inventario: make([]Carta, 0, 64),
	}
	s.adicionarCliente(cliente)
	defer s.removerCliente(cliente)

	fmt.Printf("[SERVIDOR] nova conexão %s\n", cliente.Nome)
	go s.clienteWriter(cliente)
	s.clienteReader(cliente)
}

func (s *Servidor) clienteWriter(c *Cliente) {
	for msg := range c.Mailbox {
		if err := c.Encoder.Encode(msg); err != nil {
			fmt.Printf("[SERVIDOR] erro de escrita para %s: %v\n", c.Nome, err)
			return
		}
	}
}

func (s *Servidor) clienteReader(cliente *Cliente) {
	decoder := json.NewDecoder(cliente.Conn)
	for {
		var msg protocolo.Mensagem
		if err := decoder.Decode(&msg); err != nil {
			if err == io.EOF {
				return
			}
			return
		}

		switch msg.Comando {
		case "LOGIN":
			var dadosLogin protocolo.DadosLogin
			if err := json.Unmarshal(msg.Dados, &dadosLogin); err == nil && dadosLogin.Nome != "" {
				cliente.Nome = dadosLogin.Nome
				fmt.Printf("[SERVIDOR] %s fez login como '%s'\n", cliente.Conn.RemoteAddr().String(), cliente.Nome)
			}

		case "ENTRAR_NA_FILA":
			s.entrarFila(cliente)

		case "JOGAR_CARTA":
    if cliente.Sala == nil {
        break
    }
    switch cliente.Sala.Estado {
    case "LOBBY", "FINALIZADO":
        // no lobby (ou após o fim), /jogar marca o jogador como "pronto".
        cliente.Sala.marcarProntoEIniciarSePossivel(cliente)
    case "JOGANDO":
        // durante a partida, /jogar revela a próxima carta do topo.
        cliente.Sala.processarJogada(cliente)
    }


		case "COMPRAR_PACOTE":
			// Permitido somente fora da partida (LOBBY/FINALIZADO)
			if cliente.Sala != nil && cliente.Sala.Estado == "JOGANDO" {
				s.enviar(cliente, protocolo.Mensagem{
					Comando: "ERRO",
					Dados:   mustJSON(protocolo.DadosErro{Mensagem: "Você só pode comprar pacotes fora da partida."}),
				})
				break
			}
			var req protocolo.ComprarPacoteReq
			_ = json.Unmarshal(msg.Dados, &req)
			if req.Quantidade <= 0 {
				req.Quantidade = 1
			}
			s.packReqs <- packReq{cli: cliente, quantidade: req.Quantidade}

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
		}
	}
}

/* ====================== Matchmaking ====================== */

func (s *Servidor) entrarFila(cliente *Cliente) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
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
		ID:           salaID,
		Jogadores:    []*Cliente{j1, j2},
		Estado:       "LOBBY",
		Baralhos:     make(map[string][]Carta),
		CartasNaMesa: make(map[string]Carta),
		Pontos:       make(map[string]int),
		Prontos:      make(map[string]bool),
	}
	s.salas[salaID] = novaSala
	j1.Sala, j2.Sala = novaSala, novaSala

	fmt.Printf("[SERVIDOR] Sala '%s' criada: %s vs %s (LOBBY)\n", salaID, j1.Nome, j2.Nome)

	// Notifica os dois + mostra comandos (cliente imprime)
	d1 := protocolo.DadosPartidaEncontrada{SalaID: salaID, OponenteNome: j2.Nome}
	d2 := protocolo.DadosPartidaEncontrada{SalaID: salaID, OponenteNome: j1.Nome}
	s.enviar(j1, protocolo.Mensagem{Comando: "PARTIDA_ENCONTRADA", Dados: mustJSON(d1)})
	s.enviar(j2, protocolo.Mensagem{Comando: "PARTIDA_ENCONTRADA", Dados: mustJSON(d2)})
}

/* ====================== Sala: lobby e jogo ====================== */

func (sala *Sala) marcarProntoEIniciarSePossivel(cli *Cliente) {
    // Marca o jogador como pronto no lobby. Quando os dois estiverem prontos, inicia a partida.
    sala.mutex.Lock()

    // Se a sala estava finalizada, resetamos para lobby (para poder recomeçar)
    if sala.Estado == "FINALIZADO" {
        sala.Estado = "LOBBY"
        sala.Baralhos = make(map[string][]Carta)
        sala.CartasNaMesa = make(map[string]Carta)
        sala.Pontos = make(map[string]int)
        sala.Prontos = make(map[string]bool)
    }
    if sala.Prontos == nil {
        sala.Prontos = make(map[string]bool)
    }

    sala.Prontos[cli.Nome] = true
    ready1 := sala.Prontos[sala.Jogadores[0].Nome]
    ready2 := sala.Prontos[sala.Jogadores[1].Nome]

    sala.mutex.Unlock()

    if ready1 && ready2 {
        // ambos prontos → inicia a partida
        sala.iniciarPartida()
    } else {
        // avisa que um já está pronto e aguarda o outro
        sala.enviarAtualizacaoJogo(
            fmt.Sprintf("%s está pronto. Aguardando oponente usar /jogar.", cli.Nome),
            "",
        )
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

func (sala *Sala) enviarAtualizacaoJogo(mensagem, vencedorRodada string) {
	contagem := map[string]int{}
	ultima := map[string]Carta{}
	for _, p := range sala.Jogadores {
		contagem[p.Nome] = len(sala.Baralhos[p.Nome])
		if c, ok := sala.CartasNaMesa[p.Nome]; ok {
			ultima[p.Nome] = c
		}
	}
	dados := protocolo.DadosAtualizacaoJogo{
		MensagemDoTurno: mensagem,
		ContagemCartas:  contagem,
		UltimaJogada:    ultima,
		VencedorRodada:  vencedorRodada,
	}
	sala.broadcast(nil, protocolo.Mensagem{Comando: "ATUALIZACAO_JOGO", Dados: mustJSON(dados)})
}

/* ====================== Jogo ====================== */

// iniciarPartida monta 10 cartas por jogador a partir do INVENTÁRIO.
func (sala *Sala) iniciarPartida() {
	sala.Baralhos = make(map[string][]Carta)
	sala.CartasNaMesa = make(map[string]Carta)
	sala.Pontos = make(map[string]int)
	for _, p := range sala.Jogadores {
		sala.Baralhos[p.Nome] = montarBaralhoDe10(p.Inventario)
		sala.Pontos[p.Nome] = 0
	}
	sala.Estado = "JOGANDO"
	sala.enviarAtualizacaoJogo("Partida iniciada! Use /jogar para revelar suas cartas.", "")
	fmt.Printf("[SALA %s] partida iniciada (%s vs %s)\n", sala.ID, sala.Jogadores[0].Nome, sala.Jogadores[1].Nome)
}

func (sala *Sala) processarJogada(jogador *Cliente) {
	sala.mutex.Lock()
	defer sala.mutex.Unlock()

	if sala.Estado != "JOGANDO" {
		return
	}
	// já jogou nesta rodada?
	if _, ok := sala.CartasNaMesa[jogador.Nome]; ok {
		return
	}
	deck := sala.Baralhos[jogador.Nome]
	if len(deck) == 0 {
		return
	}
	carta := deck[0]
	sala.Baralhos[jogador.Nome] = deck[1:]
	sala.CartasNaMesa[jogador.Nome] = carta

	// se os dois já jogaram, resolve a rodada
	if len(sala.CartasNaMesa) == 2 {
		p1 := sala.Jogadores[0]
		p2 := sala.Jogadores[1]
		c1 := sala.CartasNaMesa[p1.Nome]
		c2 := sala.CartasNaMesa[p2.Nome]
		venc := "EMPATE"
		if c1.Valor > c2.Valor {
			venc = p1.Nome
			sala.Pontos[p1.Nome]++
		} else if c2.Valor > c1.Valor {
			venc = p2.Nome
			sala.Pontos[p2.Nome]++
		}

		sala.enviarAtualizacaoJogo("Rodada finalizada!", venc)
		sala.CartasNaMesa = make(map[string]Carta)

		// fim da partida: ambos decks chegaram a 0 após n rodadas (10)
		if len(sala.Baralhos[p1.Nome]) == 0 && len(sala.Baralhos[p2.Nome]) == 0 {
			vencedor := "EMPATE"
			if sala.Pontos[p1.Nome] > sala.Pontos[p2.Nome] {
				vencedor = p1.Nome
			} else if sala.Pontos[p2.Nome] > sala.Pontos[p1.Nome] {
				vencedor = p2.Nome
			}
			sala.Estado = "FINALIZADO"
			sala.broadcast(nil, protocolo.Mensagem{
				Comando: "FIM_DE_JOGO",
				Dados:   mustJSON(protocolo.DadosFimDeJogo{VencedorNome: vencedor}),
			})
			// volta para o lobby: comprar pacotes e reiniciar quando quiser
		} else {
			// próxima rodada
			sala.enviarAtualizacaoJogo("Nova rodada: use /jogar.", "")
		}
	} else {
		sala.enviarAtualizacaoJogo("Aguardando o oponente...", "")
	}
}

/* ====================== Utilidades ====================== */

func (s *Servidor) adicionarCliente(c *Cliente) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if s.clientes == nil {
		s.clientes = make(map[net.Conn]*Cliente)
	}
	s.clientes[c.Conn] = c
}

func (s *Servidor) removerCliente(c *Cliente) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	delete(s.clientes, c.Conn)
	close(c.Mailbox)
}

func (s *Servidor) enviar(cli *Cliente, msg protocolo.Mensagem) {
	select {
	case cli.Mailbox <- msg:
	default:
		fmt.Printf("[SERVIDOR] mailbox cheia de %s, descartando\n", cli.Nome)
	}
}

func mustJSON(v any) []byte {
	b, _ := json.Marshal(v)
	return b
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
	// Quantidades base (ajuste à vontade)
	qtd := map[string]int{"C": 200, "U": 80, "R": 40, "L": 10}
	out := map[string][]Carta{"C": {}, "U": {}, "R": {}, "L": {}}
	for rar, n := range qtd {
		for i := 0; i < n; i++ {
			val := valorPorRaridade(rar)
			out[rar] = append(out[rar], Carta{
				ID:       novoID(),
				Nome:     nomePorValor(val),
				Valor:    val,
				Raridade: rar,
			})
		}
	}
	return out
}

func valorPorRaridade(r string) int {
	switch r {
	case "L":
		return 11 + rand.Intn(3) // 11..13
	case "R":
		return 8 + rand.Intn(5) // 8..12
	case "U":
		return 5 + rand.Intn(5) // 5..9
	default:
		return 2 + rand.Intn(6) // 2..7
	}
}

func nomePorValor(v int) string {
	switch v {
	case 11:
		return "Valete"
	case 12:
		return "Dama"
	case 13:
		return "Rei"
	default:
		return fmt.Sprintf("%d", v)
	}
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

// monta 10 cartas a partir do inventário do jogador; se faltar, completa com comuns neutras
func montarBaralhoDe10(inv []Carta) []Carta {
	deck := make([]Carta, 0, 10)
	if len(inv) > 0 {
		// amostragem aleatória sem ordenação especial; pode ponderar por raridade
		idxs := rand.Perm(len(inv))
		for i := 0; i < len(inv) && len(deck) < 10; i++ {
			deck = append(deck, inv[idxs[i]])
		}
	}
	// completa com comuns aleatórias “neutras”
	for len(deck) < 10 {
		v := 2 + rand.Intn(6)
		deck = append(deck, Carta{ID: "neutro", Nome: nomePorValor(v), Valor: v, Raridade: "C"})
	}
	// embaralha
	rand.Shuffle(len(deck), func(i, j int) { deck[i], deck[j] = deck[j], deck[i] })
	return deck
}
