// Todo arquivo Go executável precisa estar no "pacote principal" (package main).
// Isso diz ao compilador Go: "Transforme este código em um programa .exe que eu possa rodar".
package main

// A seção 'import' é onde listamos todas as "caixas de ferramentas" (pacotes ou bibliotecas)
// que nosso programa vai precisar para funcionar.
import (
	// "fmt" (pronuncia-se "fumpt") é a caixa de ferramentas para tudo que é relacionado a
	// texto formatado. Usamos para imprimir mensagens no console (na tela do terminal).
	"fmt"

	// "net" é a caixa de ferramentas para tudo relacionado a redes. É aqui que estão
	// as funções para criar servidores, clientes, lidar com conexões, etc.
	"net"
)

// --- A Lógica de Atendimento de UM Cliente ---

// Aqui definimos uma função chamada 'handleConnection'. Uma função é um bloco de código
// reutilizável que realiza uma tarefa específica.
// Esta função será o "roteiro de atendimento" para cada cliente que se conectar.
// Ela recebe um argumento chamado 'conn' do tipo 'net.Conn'.
// 'net.Conn' é o objeto que representa a conexão individual com um cliente (o "telefone"
// que nos conecta diretamente a um cliente específico).
func handleConnection(conn net.Conn) {

	// 'defer' é uma palavra-chave especial do Go. Ela agenda a execução de um comando
	// para o exato momento em que a função estiver prestes a terminar, não importa como.
	// Aqui, estamos garantindo que a conexão com o cliente ('conn.Close()') será sempre
	// fechada no final, evitando que conexões fiquem "vazando". É uma boa prática de limpeza.
	defer conn.Close()

	// fmt.Printf() é uma função que imprime um texto formatado na tela.
	// O '%s' é um "espaço reservado" que será substituído pelo texto que vem depois.
	// O '\n' significa "nova linha", para que a próxima impressão comece na linha de baixo.
	// conn.RemoteAddr().String() pega o endereço do cliente conectado (IP e porta) e o
	// converte para texto, para podermos visualizá-lo.
	fmt.Printf("[SERVIDOR] Nova conexão de %s\n", conn.RemoteAddr().String())

	// Um "buffer" é uma área de memória temporária para guardar dados. A comunicação de rede
	// acontece através de um fluxo de bytes (dados brutos). Precisamos de um lugar para
	// armazenar esses bytes quando eles chegam.
	// 'make([]byte, 1024)' cria uma "fatia de bytes" (um vetor de bytes) com capacidade
	// para armazenar 1024 bytes.
	buffer := make([]byte, 1024)

	// 'for {}' é a forma de criar um loop infinito em Go. O código dentro das chaves
	// continuará se repetindo para sempre.
	// O objetivo deste loop é continuar "ouvindo" o que este cliente específico tem a dizer,
	// até que ele se desconecte.
	for {
		// conn.Read() é a função que tenta ler dados da conexão.
		// Esta é uma chamada "bloqueante" PARA ESTA GOROUTINE. A execução desta função
		// PARA aqui e fica ESPERANDO até que o cliente envie alguma mensagem.
		// Ela retorna duas coisas:
		// 1. 'n': a quantidade de bytes que foram realmente lidos.
		// 2. 'err': um possível erro que tenha acontecido. Se tudo correu bem, 'err' será 'nil'.
		// 'nil' em Go é o mesmo que 'None' em Python ou 'null' em outras linguagens.
		// O operador ':=' é uma forma curta de declarar e atribuir valor a uma nova variável.
		n, err := conn.Read(buffer)
		
        // Em Go, o tratamento de erros é feito de forma explícita. Após quase toda operação
        // que pode falhar, nós verificamos se a variável 'err' é diferente de 'nil'.
		// Se 'err' não for 'nil', significa que algo deu errado (ex: o cliente fechou a conexão).
		if err != nil {
			// Se houve um erro, imprimimos uma mensagem informando que a conexão foi perdida.
			fmt.Printf("[SERVIDOR] Conexão com %s perdida: %s\n", conn.RemoteAddr().String(), err)
			
			// 'return' encerra a execução desta função ('handleConnection'). Como cada cliente
			// é gerenciado por uma goroutine própria, isso encerra a goroutine daquele cliente.
			return
		}

		// Aqui, estamos convertendo os bytes recebidos para um formato de texto legível (string).
		// A sintaxe 'buffer[:n]' significa: "pegue a fatia 'buffer', do início até a posição 'n'".
		// Isso é crucial para usarmos apenas os dados que foram lidos, e não o lixo que
		// possa ter ficado no resto do buffer.
		fmt.Printf("[SERVIDOR] Recebido de %s: %s\n", conn.RemoteAddr().String(), string(buffer[:n]))

		// conn.Write() envia dados de volta para o cliente.
		// Estamos enviando de volta exatamente a mesma fatia de bytes que recebemos,
		// criando assim o nosso "servidor de eco".
		conn.Write(buffer[:n])
	}
}

// A função 'main' é especial. É o ponto de partida, a primeira coisa que é executada
// quando você roda o programa.
func main() {
    fmt.Println("--- EXECUTANDO O CÓDIGO DO SERVIDOR ---")

	// Uma variável para guardar o endereço e a porta que nosso servidor usará.
	// O ':' no início significa "ouça em todos os endereços de rede desta máquina".
	endereco := ":65432"

	// net.Listen() é a função que cria o nosso servidor. Ela faz duas coisas:
	// 1. "Amarra" (bind) nosso programa ao endereço e porta especificados.
	// 2. Coloca o programa para "ouvir" (listen) por tentativas de conexão de clientes.
	listener, err := net.Listen("tcp", endereco)
	if err != nil {
		fmt.Printf("[SERVIDOR] Erro fatal ao iniciar: %s\n", err)
		return
	}
	defer listener.Close()
	fmt.Printf("[SERVIDOR] Servidor ouvindo na porta %s\n", endereco)

	// Outro loop infinito. Este é o loop principal do servidor. Sua única função
	// é esperar por NOVOS clientes que queiram se conectar.
	for {
		// listener.Accept() é a chamada que BLOQUEIA o programa principal.
		// Ele para aqui e fica esperando até que um novo cliente se conecte.
		// Quando um cliente se conecta, ele retorna o objeto 'conn' para essa conexão.
		conn, err := listener.Accept()
		if err != nil {
			fmt.Printf("[SERVIDOR] Erro ao aceitar nova conexão: %s\n", err)
			continue // 'continue' ignora o resto do loop e volta para o topo.
		}

		// A MÁGICA DA CONCORRÊNCIA EM GO!
		// A palavra-chave 'go' diz ao Go Runtime: "Execute a função 'handleConnection'
		// em uma nova GOROUTINE (um 'ajudante' super leve) e NÃO ESPERE ela terminar".
		// O programa principal dispara essa nova tarefa em segundo plano e volta
		// imediatamente para o topo do loop 'for' para esperar o próximo cliente.
		// É assim que o servidor consegue atender múltiplos clientes ao mesmo tempo.
		go handleConnection(conn)
	}
}