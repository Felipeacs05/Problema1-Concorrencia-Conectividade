// servidor/main.go

// Todo arquivo Go começa declarando a qual "pacote" ele pertence.
// 'package main' é especial: significa que este arquivo pode ser compilado
// em um programa executável.
package main

// A seção 'import' declara quais "bibliotecas" (pacotes) nosso código vai usar.
import (
	"fmt" // Pacote para formatação de texto (como o 'print' do Python)
	"net" // O pacote principal de rede do Go, contém todas as ferramentas para sockets.
)

// Esta é a função que cuidará da comunicação com UM ÚNICO cliente.
// Em Go, nós a executaremos em uma "goroutine" para cada cliente.
func handleConnection(conn net.Conn) {
	// A palavra 'defer' é uma particularidade do Go. Ela agenda a execução de uma
	// função para o momento EXATO em que a função atual estiver prestes a terminar.
	// Aqui, estamos garantindo que a conexão com o cliente será fechada, não importa o que aconteça.
	// É o equivalente ao bloco 'with conn:' do Python.
	defer conn.Close()

	// Imprime no console do servidor que um novo cliente se conectou.
	// conn.RemoteAddr() retorna o endereço IP e a porta do cliente.
	fmt.Printf("Nova conexão de %s\n", conn.RemoteAddr().String())

	// Um buffer é um pedaço de memória temporário para armazenar dados.
	// Aqui, criamos um espaço para guardar até 1024 bytes de dados vindos da rede.
	buffer := make([]byte, 1024)

	// 'for {}' é um loop infinito, para que o servidor continue ouvindo este cliente.
	for {
		// conn.Read() tenta ler dados da conexão e colocá-los no nosso 'buffer'.
		// Esta função "bloqueia" a EXECUÇÃO DESTA GOROUTINE até que dados cheguem.
		// Ela retorna 'n', o número de bytes que foram realmente lidos, e 'err', um possível erro.
		n, err := conn.Read(buffer)
		
        // O tratamento de erros em Go é explícito. Sempre verificamos se 'err' não é nulo.
		// Se houver um erro (ex: o cliente se desconectou), imprimimos e saímos da função.
		if err != nil {
			fmt.Printf("Conexão com %s perdida: %s\n", conn.RemoteAddr().String(), err)
			return // 'return' encerra a função handleConnection e a goroutine.
		}

		// Imprime o que foi recebido. Convertemos os bytes lidos para string.
		fmt.Printf("Recebido de %s: %s\n", conn.RemoteAddr().String(), string(buffer[:n]))

		// conn.Write() envia dados de volta para o cliente.
		// Estamos enviando de volta exatamente os 'n' bytes que recebemos, criando o "eco".
		conn.Write(buffer[:n])
	}
}

// A função 'main' é o ponto de entrada de todo programa executável em Go.
func main() {
	// Define a porta em que o servidor vai operar. O ':' antes do número é uma
	// sintaxe que significa "em todos os endereços de rede disponíveis nesta máquina".
	endereco := ":65432"

	// net.Listen() abre o socket e o prepara para ouvir por conexões TCP.
	// É o equivalente a 'socket.bind()' + 'socket.listen()' do Python em um só passo.
	listener, err := net.Listen("tcp", endereco)
	if err != nil {
		fmt.Printf("Erro fatal ao iniciar o servidor: %s\n", err)
		return
	}
	defer listener.Close() // Garante que o servidor seja desligado ao final.
	fmt.Printf("Servidor ouvindo na porta %s\n", endereco)

	// Loop infinito para que o servidor continue aceitando novos clientes para sempre.
	for {
		// listener.Accept() "bloqueia" a função main, esperando por um novo cliente.
		// Quando um cliente se conecta, ele retorna 'conn', a conexão com aquele cliente.
		conn, err := listener.Accept()
		if err != nil {
			fmt.Printf("Erro ao aceitar nova conexão: %s\n", err)
			continue // 'continue' pula para a próxima iteração do loop.
		}

		// --- A GRANDE MÁGICA DA CONCORRÊNCIA EM GO ---
		// A palavra-chave 'go' inicia a execução da função 'handleConnection' em uma
		// nova GOROUTINE.
		// A beleza disso é que o programa NÃO ESPERA a função terminar. Ele dispara
		// a goroutine para cuidar do cliente em segundo plano e volta imediatamente
		// para o topo do loop 'for' para esperar pelo PRÓXIMO cliente.
		//
		// >>> DIFERENÇA DO PYTHON: Em Python, precisaríamos de mais código:
		// thread = threading.Thread(target=handle_client, args=(conn, addr))
		// thread.start()
		// Em Go, a concorrência é uma cidadã de primeira classe da linguagem.
		go handleConnection(conn)
	}
}