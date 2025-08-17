// cliente/main.go
package main

import (
	"fmt"
	"net"
	"strconv" // Pacote para converter tipos (ex: número para texto)
	"time"    // Pacote para lidar com tempo e duração
)

func main() {
	// O endereço do servidor. 'servidor' é o nome que daremos no docker-compose.
	// Se fosse rodar localmente, seria 'localhost:65432'.
	endereco := "servidor:65432"

	// net.Dial() tenta se conectar a um endereço. É o 'socket.connect()' do Python.
	conn, err := net.Dial("tcp", endereco)
	if err != nil {
		fmt.Printf("Não foi possível conectar ao servidor: %s\n", err)
		return
	}
	defer conn.Close()
	fmt.Printf("Conectado ao servidor em %s\n", endereco)

	// 1. ANOTAR TEMPO INICIAL
	tempoInicial := time.Now() // Pega o objeto de tempo atual com alta precisão.

	// 2. ENVIAR A MENSAGEM
	// Convertemos o tempo para um número (nanossegundos desde 1970) e depois para texto.
	mensagem := strconv.FormatInt(tempoInicial.UnixNano(), 10)
	
    // conn.Write() envia os dados. A conversão '[]byte(mensagem)' é o
	// equivalente ao '.encode()' do Python.
	conn.Write([]byte(mensagem))

	// 3. ESPERAR PELO ECO
	buffer := make([]byte, 1024)
	_, err = conn.Read(buffer) // Espera a resposta do servidor.
	if err != nil {
		fmt.Printf("Erro ao ler resposta do servidor: %s\n", err)
		return
	}

	// 4. CALCULAR LATÊNCIA
	// >>> DIFERENÇA DO PYTHON: O Go tem uma forma mais elegante de medir duração.
	// 'time.Since(tempoInicial)' calcula automaticamente a diferença entre o
	// tempo atual e o 'tempoInicial' que guardamos.
	latencia := time.Since(tempoInicial)

	fmt.Println("Eco recebido do servidor.")
	fmt.Println("======================================")
	fmt.Printf("LATÊNCIA (IDA E VOLTA): %s\n", latencia)
	fmt.Println("======================================")
}