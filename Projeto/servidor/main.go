package main

import (
	"encoding/json"
	"fmt"
	"meujogo/protocolo"
	"net"
	"sync"
	"time"
)

type Sala struct {
	ID        string
	Jogadores []*Cliente
}

type Servidor struct {
	clientes map[net.Conn]*Cliente
	salas    map[string]*Sala
	filaDeEspera []*Cliente
	mutex    sync.Mutex
}

type Cliente struct {
	Conn    net.Conn
	Nome    string
	Encoder *json.Encoder
	Mailbox chan protocolo.Mensagem
	Sala *Sala
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
			var dadosLogin protocolo.DadosLogin

			err := json.Unmarshal(msg.Dados, &dadosLogin)
			if err != nil{
				fmt.Printf("[SERVIDOR] Erro ao decodificar dados de login: %s\n", err)
				continue
			}

			cliente.Nome = dadosLogin.Nome
			fmt.Printf("Novo login efetuado [JOGADOR: %s]\n", cliente.Nome)

		case "ENTRAR_NA_FILA":
			fmt.Printf("[SERVIDOR] Cliente '%s' entrou na fila de espera. \n", cliente.Nome)
			servidor.mutex.Lock()
			servidor.filaDeEspera = append(servidor.filaDeEspera, cliente)

			if len(servidor.filaDeEspera) >= 2 {
				jogador1 := servidor.filaDeEspera[0]
				jogador2 := servidor.filaDeEspera[1]
				servidor.filaDeEspera = servidor.filaDeEspera[2:]

				salaID := fmt.Sprintf("sala-%d", time.Now().UnixNano())
				novaSala := &Sala{
					ID: salaID,
					Jogadores: []*Cliente{jogador1, jogador2},
				}
				servidor.salas[salaID] = novaSala

				jogador1.Sala = novaSala
				jogador2.Sala = novaSala

				fmt.Printf("[SERVIDOR] Partida encontrada! Sala '%s' criada para '%s' e '%s'", salaID, jogador1, jogador2)

				//Notificar os jogadores
				dadosP1 := protocolo.DadosPartidaEncontrada{
					SalaID: salaID,
					OponenteNome: jogador2.Nome,
				}

				jsonDadosP1, _ := json.Marshal(dadosP1)
				msgP1 := protocolo.Mensagem{
					Comando: "PARTIDA_ENCONTRADA",
					Dados: jsonDadosP1,
				}
				jogador1.Mailbox <- msgP1

				dadosP2 := protocolo.DadosPartidaEncontrada{
					SalaID: salaID,
					OponenteNome: jogador1.Nome,
				}
				jsonDadosP2,_ := json.Marshal(dadosP2)
				msgP2 := protocolo.Mensagem{
					Comando: "PARTIDA_ENCONTRADA",
					Dados: jsonDadosP2,
				}
				jogador2.Mailbox <- msgP2
			}

			servidor.mutex.Unlock()

		case "ENVIAR_CHAT":

			var dadosChat protocolo.DadosEnviarChat

			fmt.Printf("[SERVIDOR] Chat de %s: %+v\n", cliente.Nome, string(msg.Dados))

			if err := json.Unmarshal(msg.Dados, &dadosChat); err != nil{
				fmt.Printf("[SERVIDOR] Erro ao decodificar dados do chat: %s\n", err)
				continue
			}

			servidor.broadcastChat(cliente, dadosChat.Texto)
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

	jsonDados, err := json.Marshal(dados)
			if err != nil{
				fmt.Printf("[SERVIDOR] Erro ao empacotar dados da resposta: %s\n", err)
				return
			}


	msg := protocolo.Mensagem{
		Comando: "RECEBER_CHAT",
		Dados:   jsonDados,
	}

	jsonParaDebug, _ := json.Marshal(msg)
	fmt.Printf("[SERVIDOR-DEBUG] Retransmitindo JSON: %s\n", string(jsonParaDebug))

	fmt.Printf("[SERVIDOR] Retransmitindo chat de %s para %d clientes\n", remetente.Nome, len(servidor.clientes)-1)

	for _, cliente := range servidor.clientes {
		if cliente.Conn != remetente.Conn{
			cliente.Mailbox <- msg
		}
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
		filaDeEspera: make([]*Cliente, 0),
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