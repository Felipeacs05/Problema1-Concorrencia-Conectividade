// cliente/main.go
package main

import (
	"fmt"
	"net"
	// 'strconv' é a caixa de ferramentas para conversões entre tipos de dados,
	// principalmente de e para 'string' (texto).
	"strconv"
	// 'time' é a caixa de ferramentas para tudo relacionado a tempo: pegar a hora
	// atual, medir durações, adicionar pausas, etc.
	"time"
)

func main() {
    fmt.Println("--- EXECUTANDO O CÓDIGO DO CLIENTE ---")

	// O endereço completo do servidor ao qual queremos nos conectar.
	// 'servidor' é o nome que o Docker Compose dará ao contêiner do servidor.
	// ':65432' é a porta que o servidor está ouvindo.
	endereco := "servidor:65432"

    // Adicionamos uma pausa deliberada. Em um ambiente Docker, às vezes o contêiner
    // do cliente pode iniciar um milissegundo antes do servidor estar 100% pronto
    // para aceitar conexões. Esta pausa dá uma pequena margem de segurança.
    time.Sleep(2 * time.Second)

	// net.Dial() é a função do cliente. Ela "disca" para o endereço do servidor
	// tentando estabelecer uma conexão. É uma chamada bloqueante: o programa
	// espera aqui até que a conexão seja estabelecida ou falhe.
	conn, err := net.Dial("tcp", endereco)
	if err != nil {
		fmt.Printf("[CLIENTE] Não foi possível conectar ao servidor: %s\n", err)
		return
	}
	defer conn.Close()
	fmt.Printf("[CLIENTE] Conectado ao servidor em %s\n", endereco)

	// 1. ANOTAR O TEMPO INICIAL
	// time.Now() retorna um objeto que representa o momento exato atual.
	tempoInicial := time.Now()

	// 2. PREPARAR E ENVIAR A MENSAGEM
	// Para medir a latência, vamos enviar o tempo exato em que começamos.
	// 'tempoInicial.UnixNano()' converte nosso objeto de tempo em um número inteiro
	// gigante, que representa os nanossegundos desde uma data de referência (01/01/1970).
	// 'strconv.FormatInt(..., 10)' converte esse número gigante para texto (base 10).
	mensagem := strconv.FormatInt(tempoInicial.UnixNano(), 10)
	
    // conn.Write() envia os dados pela rede.
    // '[]byte(mensagem)' converte nossa mensagem de texto para uma "fatia de bytes",
    // que é o formato que as conexões de rede entendem.
	conn.Write([]byte(mensagem))

	// 3. ESPERAR PELO ECO DO SERVIDOR
	buffer := make([]byte, 1024)
	// conn.Read() espera aqui até que o servidor envie a resposta de eco.
	_, err = conn.Read(buffer)
	if err != nil {
		fmt.Printf("[CLIENTE] Erro ao ler resposta do servidor: %s\n", err)
		return
	}

	// 4. CALCULAR A LATÊNCIA
	// 'time.Since(tempoInicial)' é uma função muito útil do Go. Ela simplesmente
	// calcula a duração entre o 'tempoInicial' que guardamos e o tempo de agora.
	// O resultado já é um objeto de "Duração", fácil de ler.
	latencia := time.Since(tempoInicial)

	fmt.Println("[CLIENTE] Eco recebido do servidor.")
	fmt.Println("======================================")
	// Imprimimos o objeto de duração. O '%s' sabe como formatar esse objeto
	// de forma legível (ex: "1.2345ms").
	fmt.Printf("LATÊNCIA (IDA E VOLTA): %s\n", latencia)
	fmt.Println("======================================")
}

// O programa cliente termina aqui, e a linha 'defer conn.Close()' é executada,
// fechando a conexão com o servidor.