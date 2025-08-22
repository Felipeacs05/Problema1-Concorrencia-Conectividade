package main

import (
	"encoding/json"
	"fmt"
	"meujogo/protocolo"
	"net"
	"sync"
)

type Sala struct {
	ID        string
	Jogadores []net.Conn
}

type Servidor struct {
	clientes map[net.Conn]*Cliente
	salas    map[string]*Sala
	mutex    sync.Mutex
}

type Cliente struct {
	Conn    net.Conn
	Nome    string
	Encoder *json.Encoder
	Mailbox chan protocolo.Mensagem
}

func (servidor *Servidor) handleConnection(conn net.Conn) {
	defer conn.Close()

	fmt.Printf("[SERVIDOR] Nova conexão de %s\n", conn.RemoteAddr().String())

	cliente := &Cliente{
		Conn:    conn,
		Nome:    conn.RemoteAddr().String(),
		Encoder: json.NewEncoder(conn),
		Mailbox: make(chan protocolo.Mensagem, 10),
	}

	servidor.mutex.Lock()
	servidor.clientes[conn] = cliente
	servidor.mutex.Unlock()

	go servidor.clienteWriter(cliente)

	servidor.clienteReader(cliente)

	servidor.mutex.Lock()
	delete(servidor.clientes, conn)
	servidor.mutex.Unlock()

}

func (servidor *Servidor) clienteReader(cliente *Cliente) {

	decoder := json.NewDecoder(cliente.Conn)

	for {
		var msg protocolo.Mensagem
		if err := decoder.Decode(&msg); err != nil {
			return
		}

		switch msg.Comando {
		case "LOGIN":
			cliente.Nome = "JogadorLogado"
		case "CRIAR_SALA":
			fmt.Println("[SERVIDOR] Comando de CRIAR_SALA recebido.")

			//Ativa mutex
			servidor.mutex.Lock()

			novaSala := &Sala{
				ID:        "sala1",
				Jogadores: []net.Conn{cliente.Conn},
			}
			servidor.salas[novaSala.ID] = novaSala

			dadosResposta := protocolo.DadosSalaCriada{
				SalaID: novaSala.ID,
			}
			resposta := protocolo.Mensagem{
				Comando: "SALA_CRIADA",
				Dados:   dadosResposta,
			}

			//Libera mutex
			servidor.mutex.Unlock()

			cliente.Mailbox <- resposta

			fmt.Printf("[SERVIDOR] Sala '%s' criada com sucesso para %s\n", novaSala.ID, cliente.Conn.RemoteAddr())

		case "ENVIAR_CHAT":
			fmt.Printf("[SERVIDOR] Chat de %s: %+v\n", cliente.Nome, msg.Dados)
			if dados, ok := msg.Dados.(map[string]interface{}); ok {
				servidor.broadcastChat(cliente, dados["texto"].(string))
			}
		default:
			fmt.Printf("[SERVIDOR] Comando desconhecido recebido: %s\n", msg.Comando)

		}
	}
}

func (servidor *Servidor) clienteWriter(cliente *Cliente) {
	for msg := range cliente.Mailbox {
		if err := cliente.Encoder.Encode(msg); err != nil {
			fmt.Printf("[SERVIDOR] Erro de escrita para %s: %s\n", cliente.Nome, err)
		}
	}
}

func (servidor *Servidor) broadcastChat(remetente *Cliente, texto string) {
	servidor.mutex.Lock()
	defer servidor.mutex.Unlock() //Para no final der unlock

	dados := protocolo.DadosReceberChat{
		NomeJogador: remetente.Nome,
		Texto:       texto,
	}

	msg := protocolo.Mensagem{
		Comando: "RECEBER_CHAT",
		Dados:   dados,
	}

	fmt.Printf("[SERVIDOR] Retransmitindo chat de %s para %d clientes\n", remetente.Nome, len(servidor.clientes))

	for _, cliente := range servidor.clientes {
		cliente.Mailbox <- msg
	}
}

func main() {
	fmt.Println("Executando o código do servidor...")

	endereco := ":65432"

	listener, err := net.Listen("tcp", endereco)
	if err != nil {
		fmt.Printf("[SERVIDOR] Erro fatal ao iniciar: %s\n", err)
		return
	}

	servidor := &Servidor{
		clientes: make(map[net.Conn]*Cliente),
		salas:    make(map[string]*Sala),
	}

	defer listener.Close()
	fmt.Printf("[SERVIDOR] Servidor ouvindo na porta: %s\n", endereco)

	for {
		conn, err := listener.Accept()
		if err != nil {
			fmt.Printf("[SERVIDOR] Erro ao aceitar nova conexão: %s\n", err)
			continue
		}

		go servidor.handleConnection(conn)
	}
}