# -----------------------------------------------------------------------------
# TEC502 - Prova de Conceito (PoC) - Cliente para Medir Latência
# -----------------------------------------------------------------------------
# Este script implementa um cliente que:
# 1. Tenta se conectar a um servidor em um endereço e porta específicos.
# 2. Se conectar, ele anota o tempo atual.
# 3. Envia o tempo atual (timestamp) como uma mensagem para o servidor.
# 4. Espera receber uma resposta (o eco do timestamp).
# 5. Ao receber, anota o tempo final.
# 6. Calcula a diferença para determinar a latência e a exibe.
# -----------------------------------------------------------------------------

# Importa as bibliotecas 'socket' para a comunicação de rede e 'time' para
# medir o tempo e adicionar pausas.
import socket
import time

# --- Configurações do Endereço do Servidor ---

# HOST: Aqui está a "mágica" do Docker Compose. 'servidor' é o nome que demos
# ao serviço do servidor no arquivo 'docker-compose.yml'. O Docker Compose
# possui um sistema de DNS interno que automaticamente faz com que o nome 'servidor'
# aponte para o endereço IP interno do contêiner do servidor.
HOST = 'servidor'

# PORT: A porta do servidor. DEVE ser a mesma que o servidor está usando.
PORT = 65432

# --- Lógica de Conexão Robusta ---

# Em um ambiente de contêineres, o cliente pode tentar iniciar antes que o
# servidor esteja totalmente pronto. Este loop tenta conectar 10 vezes,
# com uma pausa de 1 segundo entre as tentativas, para tornar o script mais robusto.
for i in range(10):
    try:
        # Cria o objeto socket do cliente, com as mesmas especificações (IPv4, TCP).
        s = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
        
        # s.connect() tenta estabelecer a conexão com o servidor no endereço fornecido.
        # Esta também é uma chamada bloqueante: o programa espera aqui até que a
        # conexão seja estabelecida ou um erro ocorra.
        s.connect((HOST, PORT))
        
        # Se a linha acima funcionou sem erros, a conexão foi um sucesso.
        # O 'break' interrompe o loop 'for'.
        break
    except socket.error as exc:
        # Se s.connect() falhar (ex: o servidor ainda não está pronto),
        # uma exceção 'socket.error' é capturada.
        print(f"Erro ao conectar: {exc}. Tentando novamente em 1 segundo...")
        time.sleep(1) # Pausa por 1 segundo antes da próxima tentativa.
else:
    # O bloco 'else' de um loop 'for' em Python só é executado se o loop
    # terminar "naturalmente", ou seja, sem ser interrompido por um 'break'.
    # Se chegarmos aqui, significa que todas as 10 tentativas falharam.
    print("Não foi possível conectar ao servidor após múltiplas tentativas. Abortando.")
    exit() # Encerra o script.

# --- Lógica Principal do Cliente ---

# O 'with' garante que o socket 's' seja fechado ao final da comunicação.
with s:
    print(f"Conectado ao servidor em {HOST}:{PORT}")
    
    # 1. ANOTAR TEMPO INICIAL:
    # time.time() retorna o número de segundos desde a "Epoch" (01/01/1970),
    # como um número de ponto flutuante (float) de alta precisão.
    tempo_inicial = time.time()
    
    # 2. ENVIAR A MENSAGEM:
    # Convertemos o número float do timestamp para uma string, pois só podemos
    # enviar dados que podem ser codificados em bytes.
    mensagem = str(tempo_inicial)
    print(f"Enviando timestamp: {mensagem}")
    
    # s.sendall() envia a mensagem para o servidor.
    # .encode() converte a string em um objeto de bytes, que é o formato
    # necessário para transmissão pela rede. O padrão é 'utf-8'.
    s.sendall(mensagem.encode())
    
    # 3. ESPERAR PELO ECO:
    # s.recv(1024) é a chamada BLOQUEANTE que espera pela resposta do servidor.
    # O programa para aqui até receber os dados de volta.
    data = s.recv(1024)
    
    # 4. ANOTAR TEMPO FINAL:
    # Esta linha só é executada IMEDIATAMENTE APÓS a linha de cima (recv)
    # ser concluída.
    tempo_final = time.time()

    # 5. CALCULAR E EXIBIR A LATÊNCIA:
    # Subtraímos os tempos para obter a duração da viagem de ida e volta em segundos.
    latencia_em_segundos = tempo_final - tempo_inicial
    # Multiplicamos por 1000 para converter para milissegundos (ms), que é a
    # unidade mais comum para medir latência de rede.
    latencia_em_ms = latencia_em_segundos * 1000

# .decode() converte os bytes recebidos de volta para uma string para exibição.
print(f"Eco recebido: {data.decode()}")
print("="*30)
# Exibe o resultado final formatado.
# A formatação ':.2f' limita o número de casas decimais a 2, para melhor leitura.
print(f"LATÊNCIA (IDA E VOLTA): {latencia_em_ms:.2f} ms")
print("="*30)