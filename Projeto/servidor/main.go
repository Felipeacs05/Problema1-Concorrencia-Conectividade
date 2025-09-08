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
	clientes     map[net.Conn]*Cliente
	salas        map[string]*Sala
	filaDeEspera []*Cliente
	mutex        sync.Mutex

	// ==== Estoque global de pacotes (actor) ====
	packReqs chan packReq
	estoque  map[string][]Carta // raridade -> cartas disponíveis (IDs únicos)
	packSize int                // 10 cartas por /buy_pack
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

		// Carrega cartas existentes do JSON
		cartasExistentes := carregarCartasJSON(req.cli)
		// Adiciona novas cartas
		cartasExistentes = append(cartasExistentes, cartas...)
		// Salva todas as cartas no JSON
		salvarCartasJSON(req.cli, cartasExistentes)
		// Atualiza inventário em memória
		req.cli.Inventario = cartasExistentes

		// Computa estoque restante
		rest := 0
		for _, v := range s.estoque {
			rest += len(v)
		}

		s.enviar(req.cli, protocolo.Mensagem{
			Comando: "PACOTE_RESULTADO",
			Dados:   mustJSON(protocolo.ComprarPacoteResp{Cartas: cartas, EstoqueRestante: rest}),
		})

		// Mostra cartas detalhadas após compra e marca como pronto
		mostrarCartasDetalhadas(req.cli)
		if req.cli.Sala != nil {
			req.cli.Sala.marcarCompraEIniciarSePossivel(req.cli)
		}
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
		Conn:       conn,
		Nome:       conn.RemoteAddr().String(),
		Encoder:    json.NewEncoder(conn),
		Mailbox:    make(chan protocolo.Mensagem, 32),
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
			// Permitido somente fora da partida
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
	s.salas[salaID] = novaSala
	j1.Sala, j2.Sala = novaSala, novaSala

	fmt.Printf("[SERVIDOR] Sala '%s' criada: %s vs %s (LOBBY)\n", salaID, j1.Nome, j2.Nome)

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

func (sala *Sala) enviarAtualizacaoJogo(mensagem, vencedorJogada, vencedorRodada string) {
	sala.mutex.Lock()
	dados := sala.criarAtualizacaoJogo(mensagem, vencedorJogada, vencedorRodada)
	sala.mutex.Unlock()

	sala.broadcast(nil, protocolo.Mensagem{
		Comando: "ATUALIZACAO_JOGO",
		Dados:   mustJSON(dados),
	})
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

	sala.enviarAtualizacaoJogo("[SISTEMA] PARTIDA INICIADA, cada jogador deve digitar /jogar <nomeCarta> para fazer a jogada", "", "")
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

	// Encontra a carta no inventário do jogador (por NOME)
	var carta Carta
	var cartaIndex = -1
	// normaliza entrada: remove espaços, remove o trecho a partir de '('
	entrada := strings.TrimSpace(cartaID)
	if idx := strings.Index(entrada, "("); idx >= 0 {
		entrada = strings.TrimSpace(entrada[:idx])
	}
	entradaLower := strings.ToLower(entrada)
	for i, c := range jogador.Inventario {
		nomeLower := strings.ToLower(strings.TrimSpace(c.Nome))
		if nomeLower == entradaLower {
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
		sala.enviarAtualizacaoJogo("Próxima jogada - Use /jogar <nomeCarta>", "", "")

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
	sala.enviarAtualizacaoJogo("Nova jogada - Use /jogar <nomeCarta>", "", "")
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

/* ===== Funções auxiliares da sala ===== */

// reiniciarSala limpa todos os dados da sala para uma nova partida
func (sala *Sala) reiniciarSala() {
	sala.Estado = "AGUARDANDO_PRONTOS"
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
			Dados:   mustJSON(protocolo.DadosErro{Mensagem: "Você não possui cartas. Use /buy_carta para comprar."}),
		}
		return
	}

	mensagem := "\n=== SUAS CARTAS ===\n"
	for i, carta := range cartas {
		mensagem += fmt.Sprintf("%d. %s %s (Poder: %d, Raridade: %s)\n",
			i+1, carta.Nome, carta.Naipe, carta.Valor, carta.Raridade)
	}
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

	// 60 cartas com nomes específicos
	nomesCartas := []string{
		"Dragão de Fogo", "Guerreiro Lendário", "Mago Supremo", "Anjo Guardião", "Demônio das Trevas",
		"Fênix Renascida", "Titan de Pedra", "Sereia Encantadora", "Lobo Selvagem", "Águia Real",
		"Leão Majestoso", "Tigre Feroz", "Urso Poderoso", "Raposa Astuta", "Coruja Sábia",
		"Serpente Venenosa", "Escorpião Mortal", "Aranha Gigante", "Besouro Dourado", "Borboleta Mágica",
		"Unicórnio Puro", "Pegasus Alado", "Griffin Nobre", "Hipogrifo Real", "Quimera Terrível",
		"Minotauro Feroz", "Ciclope Gigante", "Sátiro Dançante", "Ninfas da Floresta", "Fadas do Bosque",
		"Elfo Arqueiro", "Anão Ferreiro", "Orc Guerreiro", "Goblin Trapaceiro", "Troll Montanhês",
		"Gigante de Gelo", "Elemental de Fogo", "Elemental de Água", "Elemental de Terra", "Elemental de Ar",
		"Espírito da Luz", "Sombra das Trevas", "Fantasma Vingativo", "Zumbi Decadente", "Esqueleto Guerreiro",
		"Vampiro Sedutor", "Lobisomem Selvagem", "Bruxa Malvada", "Bruxo Sábio", "Necromante Sinistro",
		"Paladino Justo", "Cavaleiro Nobre", "Arqueiro Élfico", "Ladrão Astuto", "Assassino Silencioso",
		"Bardo Inspirador", "Clerigo Divino", "Druida Natural", "Monge Pacífico", "Barbaro Feroz",
	}

	naipes := []string{"♠", "♥", "♦", "♣"}
	out := map[string][]Carta{"C": {}, "U": {}, "R": {}, "L": {}}

	// Distribui as 60 cartas entre as raridades
	for i, nome := range nomesCartas {
		var raridade string
		if i < 30 { // 30 comuns
			raridade = "C"
		} else if i < 45 { // 15 incomuns
			raridade = "U"
		} else if i < 55 { // 10 raras
			raridade = "R"
		} else { // 5 lendárias
			raridade = "L"
		}

		poder := 1 + rand.Intn(100) // Poder de 1 a 100
		naipe := naipes[rand.Intn(len(naipes))]

		out[raridade] = append(out[raridade], Carta{
			ID:       novoID(),
			Nome:     nome,
			Naipe:    naipe,
			Valor:    poder,
			Raridade: raridade,
		})
	}
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
