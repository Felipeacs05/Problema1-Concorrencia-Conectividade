# -----------------------------------------------------------------------------
# TEC502 - Prova de Conceito (PoC) - Servidor de Eco
# -----------------------------------------------------------------------------
# Este script implementa um servidor simples que:
# 1. Espera por uma conexão de um cliente em uma porta específica.
# 2. Quando um cliente se conecta, ele entra em um loop.
# 3. Dentro do loop, ele aguarda receber uma mensagem.
# 4. Ao receber uma mensagem, ele a envia de volta (eco).
# 5. O loop termina se o cliente fechar a conexão.
# -----------------------------------------------------------------------------

# Importa a biblioteca 'socket', que é a ferramenta nativa do Python 
# para criar e gerenciar conexões de rede em baixo nível.
import socket

# --- Configurações do Endereço do Servidor ---

# HOST: Define em qual endereço de rede o servidor irá "ouvir".
# '0.0.0.0' é um endereço especial que significa "todos os endereços disponíveis".
# No contexto do Docker, isso é crucial, pois permite que o contêiner aceite
# conexões de fora dele mesmo (como de outro contêiner). Se usássemos 'localhost',
# ele só aceitaria conexões de dentro do seu próprio contêiner.
HOST = '0.0.0.0'

# PORT: Define o "número do apartamento" (a porta) onde o nosso programa
# estará esperando por conexões. Portas abaixo de 1024 são geralmente reservadas
# para serviços do sistema, então escolhemos uma porta alta e livre.
PORT = 65432

# --- Lógica Principal do Servidor ---

# O bloco 'with' é uma construção do Python que gerencia recursos automaticamente.
# Ao final do bloco, ele garantirá que o socket 's' seja fechado, mesmo que ocorram erros.
# socket.socket() cria o objeto socket.
# - socket.AF_INET: Especifica a família de endereços. AF_INET significa que usaremos
#   o protocolo de internet versão 4 (IPv4), que é o mais comum (ex: 192.168.0.1).
# - socket.SOCK_STREAM: Especifica o tipo de socket. SOCK_STREAM significa que usaremos
#   o protocolo TCP, que garante uma comunicação confiável, orientada à conexão e com
#   garantia de entrega dos pacotes na ordem correta (como uma chamada telefônica).
with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as s:
    
    # s.bind() vincula (associa) o socket que criamos ao endereço (HOST, PORT) que definimos.
    # A partir deste ponto, o nosso programa "possui" este endereço na máquina.
    s.bind((HOST, PORT))
    
    print(f"Servidor de Eco ouvindo em {HOST}:{PORT}")
    
    # s.listen() transforma este socket em um "socket de escuta". Ele começa a
    # aguardar ativamente por tentativas de conexão de clientes.
    s.listen()
    
    # s.accept() é uma chamada BLOQUEANTE. O programa para aqui e fica esperando
    # até que um cliente tente se conectar.
    # Quando um cliente se conecta, a função retorna duas coisas:
    # - conn: Um NOVO objeto socket, que é o canal de comunicação direto com aquele cliente específico.
    # - addr: O endereço (IP e porta) do cliente que se conectou.
    conn, addr = s.accept()
    
    # Usamos outro bloco 'with' para gerenciar a conexão com o cliente.
    # Quando este bloco terminar (se o cliente desconectar ou ocorrer um erro),
    # a conexão 'conn' será fechada automaticamente.
    with conn:
        print(f"Conectado por {addr}")
        
        # Inicia um loop infinito para manter a comunicação com o cliente.
        # O servidor continuará recebendo e ecoando mensagens até que o cliente se desconecte.
        while True:
            # conn.recv(1024) é outra chamada BLOQUEANTE. O programa para aqui
            # e espera até que o cliente envie alguma mensagem.
            # 1024 é o tamanho do buffer, ou seja, a quantidade máxima de bytes
            # que ele tentará ler da rede de uma só vez.
            # A função retorna os dados recebidos como um objeto de 'bytes'.
            data = conn.recv(1024)
            
            # Se conn.recv() retornar um objeto de bytes vazio, isso é um sinal de que
            # o cliente encerrou a conexão do lado dele.
            if not data:
                print(f"Cliente {addr} desconectou.")
                break # Sai do loop 'while', o que encerrará a conexão.
            
            # Decodificamos os dados recebidos para visualizá-los como texto.
            mensagem_recebida_str = data.decode()
            print(f"Recebido '{mensagem_recebida_str}', ecoando de volta...")
            
            # conn.sendall() envia os dados de volta para o cliente.
            # Usamos a variável 'data' original (em bytes) porque a comunicação
            # de sockets sempre trabalha com bytes, não com strings.
            conn.sendall(data)