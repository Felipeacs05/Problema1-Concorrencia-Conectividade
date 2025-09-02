// Projeto/servidor/main.go
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"meujogo/protocolo"
	"net"
	"sync"
	"time"
)

type Cliente struct {
	Conn    net.Conn
	Nome    string
	Encoder *json.Encoder
	Mailbox chan protocolo.Mensagem
	Sala    *Sala
}

type Sala struct {
	ID        string
	Jogadores []*Cliente
	mutex     sync.Mutex // Cada sala agora tem sua própria trava para suas operações internas.
}

type Servidor struct {
	clientes     map[net.Conn]*Cliente
	salas        map[string]*Sala
	filaDeEspera []*Cliente
	mutex        sync.Mutex // A trava global para operações globais (login, matchmaking).
}

// broadcast agora é um método da Sala e é seguro.
func (sala *Sala) broadcast(remetente *Cliente, msg protocolo.Mensagem) {
	sala.mutex.Lock()
	defer sala.mutex.Unlock()

	for _, jogador := range sala.Jogadores {
		if jogador.Conn != remetente.Conn {
			// Usamos um select para não bloquear caso a mailbox do outro jogador esteja cheia.
			select {
			case jogador.Mailbox <- msg:
			default:
				fmt.Printf("[SALA %s] Mailbox do jogador %s cheia. Mensagem descartada.\n", sala.ID, jogador.Nome)
			}
		}
	}
}

func (s *Servidor) handleConnection(conn net.Conn) {
	defer conn.Close()
	cliente := &Cliente{
		Conn:    conn,
		Nome:    conn.RemoteAddr().String(),
		Encoder: json.NewEncoder(conn),
		Mailbox: make(chan protocolo.Mensagem, 10),
	}

	s.mutex.Lock()
	s.clientes[conn] = cliente
	s.mutex.Unlock()
	fmt.Printf("[SERVIDOR] Nova conexão de %s\n", cliente.Nome)

	go s.clienteWriter(cliente)
	s.clienteReader(cliente)

	// --- Limpeza ---
	s.removerCliente(cliente)
}

func (s *Servidor) clienteReader(cliente *Cliente) {
	decoder := json.NewDecoder(cliente.Conn)
	for {
		var msg protocolo.Mensagem
		if err := decoder.Decode(&msg); err != nil {
			if err == io.EOF { return }
			fmt.Printf("[SERVIDOR] Erro de leitura de %s: %s\n", cliente.Nome, err)
			return
		}

		// A trava global é usada para operações que afetam o estado GERAL do servidor.
		s.mutex.Lock()
		switch msg.Comando {
		case "LOGIN":
			var dadosLogin protocolo.DadosLogin
			if err := json.Unmarshal(msg.Dados, &dadosLogin); err == nil {
				cliente.Nome = dadosLogin.Nome
				fmt.Printf("[SERVIDOR] Cliente %s atualizou nome para '%s'\n", cliente.Conn.RemoteAddr().String(), cliente.Nome)
			}

		case "ENTRAR_NA_FILA":
			fmt.Printf("[SERVIDOR] Cliente '%s' entrou na fila.\n", cliente.Nome)
			s.filaDeEspera = append(s.filaDeEspera, cliente)

			if len(s.filaDeEspera) >= 2 {
				s.criarSalaComJogadoresDaFila()
			}

		case "ENVIAR_CHAT":
			if cliente.Sala != nil {
				var dadosChat protocolo.DadosEnviarChat
				if err := json.Unmarshal(msg.Dados, &dadosChat); err == nil {
					dados := protocolo.DadosReceberChat{NomeJogador: cliente.Nome, Texto: dadosChat.Texto}
					jsonDados, _ := json.Marshal(dados)
					msgParaBroadcast := protocolo.Mensagem{Comando: "RECEBER_CHAT", Dados: jsonDados}
					
					// A lógica de broadcast agora é da sala, não mais global.
					// Não precisamos da trava global aqui, pois a sala tem a sua própria.
					s.mutex.Unlock() // Liberamos a trava global ANTES de chamar o broadcast da sala.
					cliente.Sala.broadcast(cliente, msgParaBroadcast)
					s.mutex.Lock() // Pegamos a trava de volta para o final do loop.
				}
			}
		}
		s.mutex.Unlock()
	}
}

func (s *Servidor) criarSalaComJogadoresDaFila() {
	// Esta função assume que o mutex global já está travado.
	jogador1 := s.filaDeEspera[0]
	jogador2 := s.filaDeEspera[1]
	s.filaDeEspera = s.filaDeEspera[2:]

	salaID := fmt.Sprintf("sala-%d", time.Now().UnixNano())
	novaSala := &Sala{
		ID:        salaID,
		Jogadores: []*Cliente{jogador1, jogador2},
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
}

func (s *Servidor) clienteWriter(cliente *Cliente) {
	for msg := range cliente.Mailbox {
		if err := cliente.Encoder.Encode(msg); err != nil {
			fmt.Printf("[SERVIDOR] Erro de escrita para %s: %s\n", cliente.Nome, err)
		}
	}
}

func (s *Servidor) removerCliente(cliente *Cliente) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	delete(s.clientes, cliente.Conn)
	// Lógica futura: notificar oponente na sala, etc.
}

func main() {
	servidor := &Servidor{
		clientes:     make(map[net.Conn]*Cliente),
		salas:        make(map[string]*Sala),
		filaDeEspera: make([]*Cliente, 0),
	}
	listener, _ := net.Listen("tcp", ":65432")
	defer listener.Close()
	fmt.Println("[SERVIDOR] Servidor ouvindo na porta :65432")

	for {
		conn, _ := listener.Accept()
		go servidor.handleConnection(conn)
	}
}