# Jogo de Cartas Multiplayer (PBL - TEC502)

Este repositório contém a implementação do Problema 1 da disciplina TEC502 - Concorrência e Conectividade, oferecida pela Universidade Estadual de Feira de Santana (UEFS).

## 📝 Descrição do Projeto

O projeto consiste no desenvolvimento de um jogo de cartas online multiplayer, focado em duelos táticos 1v1 e coleção de cartas. A arquitetura é baseada em um servidor centralizado de alta concorrência, construído em Go, que gerencia a lógica do jogo, o estado dos jogadores, o pareamento em tempo real e a distribuição de cartas.

O sistema foi projetado para suportar milhares de jogadores simultâneos, utilizando Goroutines para escalabilidade e canais para comunicação segura entre os componentes. Toda a comunicação foi implementada utilizando a biblioteca nativa de sockets TCP da linguagem Go, sem o uso de frameworks de comunicação, conforme as restrições do problema.

## ✨ Funcionalidades Implementadas

* **Servidor Concorrente de Alta Performance:** O servidor utiliza Goroutines para lidar com milhares de clientes de forma concorrente e eficiente. Emprega otimizações como *worker pools* para processamento de tarefas pesadas (compra de pacotes) e `sync.Pool` para reduzir a alocação de memória e a carga no Garbage Collector.
* **Pareamento de Partidas 1v1:** Sistema de fila automatizado que pareia jogadores para partidas únicas assim que dois deles estão disponíveis.
* **Mecânica de Jogo Completa:**
    * **Compra de Pacotes:** Jogadores podem comprar pacotes de cartas de um estoque global. O sistema garante a distribuição justa e atômica, mesmo sob alta contenção.
    * **Batalha de Cartas:** A lógica de turno permite que os jogadores joguem cartas de seu inventário. O vencedor da jogada é determinado pelo poder e naipe da carta.
* **Comunicação Robusta via Protocolo Estruturado:** A comunicação entre cliente e servidor é feita através de um protocolo customizado baseado em JSON, garantindo clareza e manutenibilidade na troca de mensagens.
* **Chat em Tempo Real:** Uma funcionalidade de chat permite que os jogadores de uma mesma sala troquem mensagens durante a partida.
* **Medição de Latência:** Os jogadores podem verificar a latência (ping) com o servidor a qualquer momento com o comando `/ping`.
* **Testes de Estresse:** O projeto inclui um cliente de teste de estresse capaz de simular milhares de conexões simultâneas para validar a estabilidade, o desempenho e a justiça do servidor sob carga pesada.
* **Ambiente Containerizado:** Todos os componentes do projeto (servidor, cliente e cliente de estresse) são executados em contêineres Docker, garantindo um ambiente de execução e teste padronizado e reprodutível.

## 🛠️ Arquitetura e Tecnologias

* **Linguagem:** Go
* **Concorrência:** Goroutines, Channels, `sync.Mutex`, `sync.Map` e `sync.Pool`.
* **Comunicação:** Sockets TCP Nativos (pacote `net`)
* **Serialização de Dados:** JSON
* **Containerização:** Docker & Docker Compose

A arquitetura segue o modelo Cliente-Servidor. O servidor (`/servidor`) é o núcleo da aplicação, mantendo o estado global, gerenciando as salas de jogo e orquestrando toda a comunicação. Os clientes (`/cliente`) são aplicações de terminal interativas que se conectam ao servidor para enviar comandos e receber atualizações de estado. O pacote `/protocolo` define as estruturas de dados compartilhadas, garantindo a consistência da comunicação.

## 🚀 Como Executar o Projeto

Para executar o projeto, você precisará ter o **Docker** e o **Docker Compose** instalados em sua máquina.

### Pré-requisitos

* Git
* Docker (`>= 20.10`)
* Docker Compose

### Passos para Execução

1.  **Clone o repositório:**
    ```bash
    git clone [https://github.com/Felipeacs05/Problema1-Concorrencia-Conectividade](https://github.com/Felipeacs05/Problema1-Concorrencia-Conectividade)
    cd Problema1-Concorrencia-Conectividade/Projeto/
    ```

2.  **Inicie o Servidor:**
    Abra um terminal na pasta raiz do projeto (`Projeto/`) e execute o comando abaixo para construir a imagem Docker do servidor e iniciá-la.
    ```bash
    docker compose up --build servidor
    ```
    Este terminal se tornará o console do servidor. Deixe-o rodando.

3.  **Conecte os Jogadores:**
    Abra **dois novos terminais** separados e navegue até a **mesma pasta** `Projeto/`. Execute os seguintes comandos (um em cada terminal):

    * **Terminal do Jogador A:**
        ```bash
        docker compose run --rm cliente
        ```
    * **Terminal do Jogador B:**
        ```bash
        docker compose run --rm cliente
        ```

4.  **Jogue a Partida:**
    Siga as instruções no terminal de cada jogador. Eles serão pareados automaticamente. Usem os comandos abaixo para interagir com o jogo.

### Comandos do Jogo

* `/comprar` - Compra um pacote de cartas para iniciar a partida.
* `/jogar <ID_da_carta>` - Joga uma carta da sua mão.
* `/cartas` - Mostra as cartas que você tem na mão.
* `/ping` - Mede sua latência com o servidor.
* `/sair` - Abandona a partida atual e volta para a fila.
* Qualquer outro texto digitado é enviado como uma mensagem de chat para o oponente.

### Executando o Teste de Estresse

Para simular uma grande quantidade de jogadores e testar a performance do servidor, execute o serviço `cliente-estresse`:

```bash
# Inicie o servidor primeiro (em modo detached)
docker compose up -d servidor

# Execute o teste de estresse
docker compose run --rm cliente-estresse
