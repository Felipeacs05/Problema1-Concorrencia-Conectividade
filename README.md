# Jogo de Cartas Multiplayer (PBL - TEC502)

Este repositório contém a implementação do Problema 1 da disciplina TEC502 - Concorrência e Conectividade, oferecida pela Universidade Estadual de Feira de Santana (UEFS).

## 📝 Descrição do Projeto

O projeto consiste no desenvolvimento de um novo jogo de cartas online multiplayer, focado em duelos táticos e coleção de cartas. A arquitetura é baseada em um servidor centralizado que gerencia a lógica do jogo, o estado dos jogadores e a comunicação entre eles em tempo real.

O sistema foi projetado para permitir que múltiplos jogadores se conectem simultaneamente e se enfrentem em partidas 1v1, com um sistema de pareamento que garante duelos únicos. Toda a comunicação foi implementada utilizando a biblioteca nativa de sockets da linguagem Go, sem o uso de frameworks de comunicação, conforme as restrições do problema.

## ✨ Funcionalidades Implementadas

  * **Servidor Concorrente:** O servidor utiliza Goroutines para lidar com múltiplos clientes de forma concorrente e eficiente, permitindo que várias interações ocorram simultaneamente.
  * **Comunicação via Protocolo Estruturado:** A comunicação entre cliente e servidor é feita através de um protocolo customizado baseado em JSON, garantindo robustez e clareza na troca de mensagens.
  * **Chat Interativo em Tempo Real:** Como prova de conceito da comunicação bidirecional, foi implementado um sistema de chat que permite que dois clientes, conectados ao servidor, troquem mensagens em tempo real.
  * **Gerenciamento de Estado:** O servidor mantém um estado centralizado dos clientes conectados, utilizando mutexes para garantir a segurança em acessos concorrentes.
  * **Ambiente Containerizado:** Todos os componentes do projeto (servidor e cliente) são executados em contêineres Docker, garantindo um ambiente de execução e teste padronizado e reprodutível.

## 🛠️ Arquitetura e Tecnologias

  * **Linguagem:** Go
  * **Concorrência:** Goroutines e Channels
  * **Comunicação:** Sockets TCP Nativos
  * **Serialização de Dados:** JSON
  * **Containerização:** Docker & Docker Compose

A arquitetura segue o modelo Cliente-Servidor. O servidor (`/servidor`) é o "cérebro" da aplicação, mantendo o estado e orquestrando a comunicação. Os clientes (`/cliente`) são aplicações de terminal interativas que se conectam ao servidor para enviar e receber informações. O pacote `/protocolo` define as estruturas de dados compartilhadas entre ambos, garantindo a consistência da comunicação.

## 🚀 Como Executar o Projeto

Para executar o projeto, você precisará ter o **Docker** e o **Docker Compose** instalados em sua máquina.

### Pré-requisitos

  * Git
  * Docker (`>= 20.10`)
  * Docker Compose

### Passos para Execução

1.  **Clone o repositório:**

    ```bash
    git clone <URL_DO_SEU_REPOSITORIO_GIT>
    cd Projeto/
    ```

2.  **Inicie o Servidor (Terminal 1):**
    Abra um terminal na pasta raiz do projeto (`Projeto/`) e execute o comando abaixo. Este comando irá construir a imagem Docker do servidor e iniciá-la.

    ```bash
    docker compose up --build servidor
    ```

    Este terminal se tornará o console do servidor. Você verá os logs de conexões e mensagens aqui. Deixe-o rodando.

3.  **Conecte o Primeiro Jogador (Terminal 2):**
    Abra um **novo** terminal, navegue até a **mesma pasta** `Projeto/` e execute o comando:

    ```bash
    docker compose run --name jogador_a cliente
    ```

    Este comando cria um contêiner interativo para o primeiro jogador. Um prompt `>` aparecerá, esperando que você digite as mensagens do chat.

4.  **Conecte o Segundo Jogador (Terminal 3):**
    Abra um **terceiro** terminal, navegue até a pasta `Projeto/` e execute o comando:

    ```bash
    docker compose run --name jogador_b cliente
    ```

    Este será o terminal do segundo jogador.

5.  **Teste o Chat:**
    Agora você pode digitar uma mensagem no terminal do Jogador A e apertar Enter. A mensagem aparecerá no terminal do Jogador B, e vice-versa.

6.  **Para Encerrar:**
    Para parar todos os contêineres e remover a rede, volte ao **Terminal 1** (o do servidor) e pressione `Ctrl + C`. Depois, para garantir que tudo seja limpo, execute:

    ```bash
    docker compose down
    ```

## 📂 Estrutura de Arquivos

```
Projeto/
├── .gitignore          # Arquivos a serem ignorados pelo Git
├── go.mod              # Define o módulo Go e suas dependências
├── docker-compose.yml  # Orquestra os serviços de servidor e cliente
├── servidor/
│   ├── main.go         # Código-fonte do servidor concorrente
│   └── Dockerfile      # Receita para construir a imagem Docker do servidor
├── cliente/
│   ├── main.go         # Código-fonte do cliente interativo
│   └── Dockerfile      # Receita para construir a imagem Docker do cliente
└── protocolo/
    └── protocolo.go    # Definições das structs (JSON) compartilhadas
```
