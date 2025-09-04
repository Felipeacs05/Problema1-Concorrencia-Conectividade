// Projeto/servidor/main.go
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

// --- Estruturas do Jogo ---
type Carta = protocolo.Carta

type Cliente struct {
	Conn    net.Conn
	Nome    string
	Encoder *json.Encoder
	Mailbox chan protocolo.Mensagem
	Sala    *Sala
}

type Sala struct {
	ID           string
	Jogadores    []*Cliente
	EstadoDoJogo string
	Baralhos     map[string][]Carta
	CartasNaMesa map[string]Carta
	mutex        sync.Mutex
}

type Servidor struct {
	clientes     map[net.Conn]*Cliente
	salas        map[string]*Sala
	filaDeEspera []*Cliente
	mutex        sync.Mutex
}

// --- Métodos da Sala (Lógica do Jogo) ---


// broadcast agora é um MÉTODO da Sala e é seguro.
func (sala *Sala) broadcast(remetente *Cliente, msg protocolo.Mensagem) {
	sala.mutex.Lock()
	defer sala.mutex.Unlock()

	for _, jogador := range sala.Jogadores {
		if jogador != remetente {
			select {
			case jogador.Mailbox <- msg:
			default:
				fmt.Printf("[SALA %s] Mailbox do jogador %s cheia. Mensagem descartada.\n", sala.ID, jogador.Nome)
			}
		}
	}
}
func (sala *Sala) notificarTodos(msg protocolo.Mensagem) {
	// Esta função não precisa de trava própria, pois será chamada por
	// funções que já detêm a trava da sala.
	for _, jogador := range sala.Jogadores {
		select {
		case jogador.Mailbox <- msg:
		default:
			fmt.Printf("[SALA %s] Mailbox do jogador %s cheia. Mensagem descartada.\n", sala.ID, jogador.Nome)
		}
	}
}

func (sala *Sala) iniciarPartida() {
	sala.mutex.Lock()
	defer sala.mutex.Unlock()

	baralho := criarBaralho()
	rand.New(rand.NewSource(time.Now().UnixNano())).Shuffle(len(baralho), func(i, j int) {
		baralho[i], baralho[j] = baralho[j], baralho[i]
	})

	p1 := sala.Jogadores[0]
	p2 := sala.Jogadores[1]
	sala.Baralhos[p1.Nome] = baralho[:26]
	sala.Baralhos[p2.Nome] = baralho[26:]
	sala.EstadoDoJogo = "JOGANDO"

	fmt.Printf("[SALA %s] Partida iniciada. %s (%d cartas) vs %s (%d cartas)\n", sala.ID, p1.Nome, len(sala.Baralhos[p1.Nome]), p2.Nome, len(sala.Baralhos[p2.Nome]))
	sala.enviarAtualizacaoJogo("Partida iniciada! É a vez de todos jogarem.", "")
}

func (sala *Sala) processarJogada(jogador *Cliente) {
	sala.mutex.Lock()
	defer sala.mutex.Unlock()

	// Verifica se o jogador já jogou nesta rodada
	if _, jaJogou := sala.CartasNaMesa[jogador.Nome]; jaJogou {
		return // Ignora a jogada se ele já jogou
	}
	if len(sala.Baralhos[jogador.Nome]) == 0 {
		return // Jogador não tem mais cartas
	}

	cartaJogada := sala.Baralhos[jogador.Nome][0]
	sala.Baralhos[jogador.Nome] = sala.Baralhos[jogador.Nome][1:]
	sala.CartasNaMesa[jogador.Nome] = cartaJogada

	fmt.Printf("[SALA %s] Jogador %s jogou %s\n", sala.ID, jogador.Nome, cartaJogada.Nome)

	// Se ambos jogaram, resolve a rodada
	if len(sala.CartasNaMesa) == 2 {
		p1 := sala.Jogadores[0]
		p2 := sala.Jogadores[1]
		cartaP1 := sala.CartasNaMesa[p1.Nome]
		cartaP2 := sala.CartasNaMesa[p2.Nome]
		vencedorRodada := ""

		if cartaP1.Valor > cartaP2.Valor {
			sala.Baralhos[p1.Nome] = append(sala.Baralhos[p1.Nome], cartaP1, cartaP2)
			vencedorRodada = p1.Nome
		} else if cartaP2.Valor > cartaP1.Valor {
			sala.Baralhos[p2.Nome] = append(sala.Baralhos[p2.Nome], cartaP1, cartaP2)
			vencedorRodada = p2.Nome
		} else {
			vencedorRodada = "EMPATE" // Cartas são descartadas
		}

		fmt.Printf("[SALA %s] Fim da rodada. Vencedor: %s. Placar: %s %d / %s %d\n", sala.ID, vencedorRodada, p1.Nome, len(sala.Baralhos[p1.Nome]), p2.Nome, len(sala.Baralhos[p2.Nome]))
		
		mensagemTurno := fmt.Sprintf("Rodada finalizada! Vencedor: %s. Joguem novamente!", vencedorRodada)
		
		// Verifica condição de vitória
		if len(sala.Baralhos[p1.Nome]) == 0 {
			sala.finalizarPartida(p2)
		} else if len(sala.Baralhos[p2.Nome]) == 0 {
			sala.finalizarPartida(p1)
		} else {
			sala.enviarAtualizacaoJogo(mensagemTurno, vencedorRodada)
			sala.CartasNaMesa = make(map[string]Carta) // Limpa a mesa para a próxima rodada
		}
	} else {
		sala.enviarAtualizacaoJogo("Aguardando oponente jogar...", "")
	}
}

func (sala *Sala) finalizarPartida(vencedor *Cliente) {
	sala.EstadoDoJogo = "FINALIZADO"
	dados := protocolo.DadosFimDeJogo{VencedorNome: vencedor.Nome}
	jsonDados, _ := json.Marshal(dados)
	msg := protocolo.Mensagem{Comando: "FIM_DE_JOGO", Dados: jsonDados}
	sala.notificarTodos(msg)
	fmt.Printf("[SALA %s] Fim de jogo! Vencedor: %s\n", sala.ID, vencedor.Nome)
}

func (sala *Sala) enviarAtualizacaoJogo(mensagemTurno string, vencedorRodada string) {
	contagem := make(map[string]int)
	ultimaJogada := make(map[string]Carta)
	for _, p := range sala.Jogadores {
		contagem[p.Nome] = len(sala.Baralhos[p.Nome])
		if carta, ok := sala.CartasNaMesa[p.Nome]; ok {
			ultimaJogada[p.Nome] = carta
		}
	}

	dados := protocolo.DadosAtualizacaoJogo{
		MensagemDoTurno: mensagemTurno,
		ContagemCartas:  contagem,
		UltimaJogada:    ultimaJogada,
		VencedorRodada:  vencedorRodada,
	}
	jsonDados, _ := json.Marshal(dados)
	msg := protocolo.Mensagem{Comando: "ATUALIZACAO_JOGO", Dados: jsonDados}
	sala.notificarTodos(msg)
}

// --- Lógica do Servidor ---

func (s *Servidor) handleConnection(conn net.Conn) {
	defer conn.Close()
	cliente := &Cliente{
		Conn:    conn,
		Nome:    conn.RemoteAddr().String(),
		Encoder: json.NewEncoder(conn),
		Mailbox: make(chan protocolo.Mensagem, 10),
	}
	s.adicionarCliente(cliente)
	defer s.removerCliente(cliente)

	fmt.Printf("[SERVIDOR] Nova conexão de %s\n", cliente.Nome)
	go s.clienteWriter(cliente)
	s.clienteReader(cliente)
}

func (s *Servidor) clienteReader(cliente *Cliente) {
	decoder := json.NewDecoder(cliente.Conn)
	for {
		var msg protocolo.Mensagem
		if err := decoder.Decode(&msg); err != nil {
			if err == io.EOF { return }
			return
		}

		s.mutex.Lock()
		switch msg.Comando {
		case "LOGIN":
			var dadosLogin protocolo.DadosLogin
			if err := json.Unmarshal(msg.Dados, &dadosLogin); err == nil {
				cliente.Nome = dadosLogin.Nome
				fmt.Printf("[SERVIDOR] Cliente %s atualizou nome para '%s'\n", cliente.Conn.RemoteAddr().String(), cliente.Nome)
			}
		case "ENTRAR_NA_FILA":
			s.filaDeEspera = append(s.filaDeEspera, cliente)
			fmt.Printf("[SERVIDOR] Cliente '%s' entrou na fila. (%d na fila)\n", cliente.Nome, len(s.filaDeEspera))
			if len(s.filaDeEspera) >= 2 {
				s.criarSalaComJogadoresDaFila()
			}
		case "JOGAR_CARTA":
			if cliente.Sala != nil && cliente.Sala.EstadoDoJogo == "JOGANDO" {
				s.mutex.Unlock()
				cliente.Sala.processarJogada(cliente)
				s.mutex.Lock()
			}
		case "ENVIAR_CHAT":
			if cliente.Sala != nil {
				var dadosChat protocolo.DadosEnviarChat
				if err := json.Unmarshal(msg.Dados, &dadosChat); err == nil {
					dados := protocolo.DadosReceberChat{NomeJogador: cliente.Nome, Texto: dadosChat.Texto}
					jsonDados, _ := json.Marshal(dados)
					msgParaBroadcast := protocolo.Mensagem{Comando: "RECEBER_CHAT", Dados: jsonDados}
					s.mutex.Unlock()
					cliente.Sala.broadcast(cliente, msgParaBroadcast)
					s.mutex.Lock()
				}
			}
		}
		s.mutex.Unlock()
	}
}

func (s *Servidor) criarSalaComJogadoresDaFila() {
	jogador1 := s.filaDeEspera[0]
	jogador2 := s.filaDeEspera[1]
	s.filaDeEspera = s.filaDeEspera[2:]

	salaID := fmt.Sprintf("sala-%d", time.Now().UnixNano())
	novaSala := &Sala{
		ID:           salaID,
		Jogadores:    []*Cliente{jogador1, jogador2},
		Baralhos:     make(map[string][]Carta),
		CartasNaMesa: make(map[string]Carta),
	}
	s.salas[salaID] = novaSala
	jogador1.Sala = novaSala
	jogador2.Sala = novaSala

	fmt.Printf("[SERVIDOR] Sala '%s' criada para '%s' e '%s'.\n", salaID, jogador1.Nome, jogador2.Nome)

	dadosP1 := protocolo.DadosPartidaEncontrada{SalaID: salaID, OponenteNome: jogador2.Nome}
	jsonDadosP1, _ := json.Marshal(dadosP1)
	msgP1 := protocolo.Mensagem{Comando: "PARTIDA_ENCONTRADA", Dados: jsonDadosP1}
	jogador1.Mailbox <- msgP1

	dadosP2 := protocolo.DadosPartidaEncontrada{SalaID: salaID, OponenteNome: jogador1.Nome}
	jsonDadosP2, _ := json.Marshal(dadosP2)
	msgP2 := protocolo.Mensagem{Comando: "PARTIDA_ENCONTRADA", Dados: jsonDadosP2}
	jogador2.Mailbox <- msgP2

	go novaSala.iniciarPartida()
}

func (s *Servidor) clienteWriter(cliente *Cliente) {
	for msg := range cliente.Mailbox {
		if err := cliente.Encoder.Encode(msg); err != nil {
			fmt.Printf("[SERVIDOR] Erro de escrita para %s: %s\n", cliente.Nome, err)
		}
	}
}

func (s *Servidor) adicionarCliente(cliente *Cliente) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.clientes[cliente.Conn] = cliente
}

func (s *Servidor) removerCliente(cliente *Cliente) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	// Lógica futura: notificar oponente na sala, remover da fila, etc.
	delete(s.clientes, cliente.Conn)
}

func criarBaralho() []Carta {
	naipes := []string{"Copas", "Ouros", "Paus", "Espadas"}
	nomes := map[int]string{11: "Valete", 12: "Dama", 13: "Rei", 14: "Ás"}
	var baralho []Carta

	for _, naipe := range naipes {
		for valor := 2; valor <= 14; valor++ {
			nome := fmt.Sprintf("%d de %s", valor, naipe)
			if v, ok := nomes[valor]; ok {
				nome = fmt.Sprintf("%s de %s", v, naipe)
			}
			baralho = append(baralho, Carta{Naipe: naipe, Valor: valor, Nome: nome})
		}
	}
	return baralho
}

func main() {
	servidor := &Servidor{
		clientes:     make(map[net.Conn]*Cliente),
		salas:        make(map[string]*Sala),
		filaDeEspera: make([]*Cliente, 0),
	}
	listener, _ := net.Listen("tcp", ":65432")
	defer listener.Close()
	fmt.Println("[SERVIDOR] Servidor de Jogo ouvindo na porta :65432")

	for {
		conn, _ := listener.Accept()
		go servidor.handleConnection(conn)
	}
}