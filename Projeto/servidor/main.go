package main

import (
	"encoding/json" 
	"fmt"
	"net"
	"meujogo/protocolo"
	"sync"
)

type Sala struct {
	ID string
	Jogadores []net.Conn
}

type Servidor struct {
	salas map[string]*Sala
	mutex sync.Mutex
}

func handleConnection(conn net.Conn, servidor *Servidor){
	defer conn.Close()

	fmt.Printf("[SERVIDOR] Nova conexão de %s\n", conn.RemoteAddr().String())

	//Receber do cliente
	decoder := json.NewDecoder(conn)

	//Enviar para o cliente
	encoder := json.NewEncoder(conn)

	for{
		var msg protocolo.Mensagem

		err := decoder.Decode(&msg)
		if err != nil {
			// Se o erro for 'io.EOF', significa que o cliente desconectou de forma limpa.
			fmt.Printf("[SERVIDOR] Conexão com %s fechada.\n", conn.RemoteAddr().String())
			return // Encerra a goroutine.
		}//

		fmt.Printf("[SERVIDOR] JSON recebido: %+v\n", msg)

		switch msg.Comando {
		case "LOGIN":
			fmt.Println("[SERVIDOR] Comando de LOGIN recebido")
		case "CRIAR_SALA":
			fmt.Println("[SERVIDOR] Comando de CRIAR_SALA recebido.")

			//Ativa mutex
			servidor.mutex.Lock()

			novaSala := &Sala {
				ID: "sala1",
				Jogadores: []net.Conn{conn},
			}
			servidor.salas[novaSala.ID] = novaSala

			dadosResposta := protocolo.DadosSalaCriada{
				SalaID: novaSala.ID,
			}

			resposta := protocolo.Mensagem{
				Comando: "SALA_CRIADA",
				Dados: dadosResposta,
			}

			err = encoder.Encode(resposta)
			if err != nil{
				fmt.Printf("[SERVIDOR] Erro ao enviar a mensagem: %s\n", err)
			}

			//Libera mutex
			servidor.mutex.Unlock()

			fmt.Printf("[SERVIDOR] Sala '%s' criada com sucesso para %s\n", novaSala.ID, conn.RemoteAddr())
		
		default:
			fmt.Printf("[SERVIDOR] Comando desconhecido recebido: %s\n", msg.Comando)	
		}

	}
}
func main(){
	fmt.Println("Executando o código do servidor...")

	endereco := ":65432"

	listener, err := net.Listen("tcp", endereco)
	if err != nil {
		fmt.Printf("[SERVIDOR] Erro fatal ao iniciar: %s\n", err)
		return
	}

	servidor := &Servidor{
		salas: make(map[string]*Sala),
	}

	defer listener.Close()
	fmt.Printf("[SERVIDOR] Servidor ouvindo na porta: %s\n", endereco)

	for {
		conn, err := listener.Accept()
		if err != nil{
			fmt.Printf("[SERVIDOR] Erro ao aceitar nova conexão: %s\n", err)
			continue
		}

		go handleConnection(conn, servidor)
	}
}