package protocolo

import "encoding/json"

// ===================== BAREMA ITEM 3: API REMOTA =====================
// Este arquivo define toda a API de comunicação entre servidor e clientes.
// Cada tipo de mensagem representa um comando específico do protocolo,
// permitindo comunicação assíncrona e estruturada entre os componentes.

// Envelope base para todas as mensagens do protocolo
// BAREMA ITEM 4: ENCAPSULAMENTO - Esta estrutura garante que todas as mensagens
// tenham um formato consistente, facilitando o parsing e validação no servidor
type Mensagem struct {
	Comando string          `json:"comando"` // Tipo da operação (LOGIN, JOGAR_CARTA, etc.)
	Dados   json.RawMessage `json:"dados"`   // Payload específico de cada comando
}

/* ===================== Cartas / Inventário ===================== */

// BAREMA ITEM 4: ENCAPSULAMENTO - Estrutura de dados para cartas do jogo
// Cada carta possui um ID único, garantindo que não haja duplicatas no estoque global.
// A raridade determina a probabilidade de aparecer em pacotes (C=70%, U=20%, R=9%, L=1%)
type Carta struct {
	ID       string `json:"id"`                 // Identificador único da carta no estoque global
	Nome     string `json:"nome"`               // Nome da carta para exibição
	Naipe    string `json:"naipe"`              // "♠", "♥", "♦", "♣" - usado para desempate
	Valor    int    `json:"valor"`              // Poder da carta (1..13, onde Ás=1, Rei=13)
	Raridade string `json:"raridade,omitempty"` // C=Comum, U=Incomum, R=Rara, L=Lendária
}

// BAREMA ITEM 8: PACOTES - Estrutura para solicitação de compra de pacotes
// O cliente pode especificar quantos pacotes deseja comprar (padrão: 1 pacote = 5 cartas)
type ComprarPacoteReq struct {
	Quantidade int `json:"quantidade"` // Quantidade de pacotes desejados (padrão: 1)
}

// BAREMA ITEM 8: PACOTES - Resposta do servidor com as cartas adquiridas
// Inclui informações sobre o estoque restante para transparência
type ComprarPacoteResp struct {
	Cartas          []Carta `json:"cartas"`          // Cartas recebidas no pacote
	EstoqueRestante int     `json:"estoqueRestante"` // Quantidade de cartas restantes no estoque global
}

/* ===================== Login / Match / Chat ===================== */

// BAREMA ITEM 7: PARTIDAS - Dados para autenticação do jogador
type DadosLogin struct {
	Nome string `json:"nome"` // Nome único do jogador no sistema
}

// BAREMA ITEM 7: PARTIDAS - Notificação de que uma partida foi encontrada
// Enviado quando o sistema de matchmaking encontra um oponente compatível
type DadosPartidaEncontrada struct {
	SalaID       string `json:"salaID"`       // ID único da sala de jogo criada
	OponenteNome string `json:"oponenteNome"` // Nome do oponente encontrado
}

// BAREMA ITEM 3: API REMOTA - Dados para envio de mensagens de chat
type DadosEnviarChat struct {
	Texto string `json:"texto"` // Conteúdo da mensagem de chat
}

// BAREMA ITEM 3: API REMOTA - Dados para jogada de carta
// O cliente especifica qual carta jogar através do seu ID único
type DadosJogarCarta struct {
	CartaID string `json:"cartaID"` // ID da carta a ser jogada
}

// BAREMA ITEM 3: API REMOTA - Dados para recebimento de mensagens de chat
// Inclui o nome do remetente para identificação
type DadosReceberChat struct {
	NomeJogador string `json:"nomeJogador"` // Nome do jogador que enviou a mensagem
	Texto       string `json:"texto"`       // Conteúdo da mensagem
}

/* ===================== Atualizações de jogo ===================== */

// BAREMA ITEM 3: API REMOTA - Estrutura principal para atualizações do estado do jogo
// Enviada para todos os jogadores sempre que há mudanças no estado da partida
type DadosAtualizacaoJogo struct {
	MensagemDoTurno string           `json:"mensagemDoTurno"` // Mensagem descritiva do que aconteceu
	ContagemCartas  map[string]int   `json:"contagemCartas"`  // nome -> cartas restantes no inventário
	UltimaJogada    map[string]Carta `json:"ultimaJogada"`    // nome -> carta recém jogada na mesa
	VencedorJogada  string           `json:"vencedorJogada"`  // nome do vencedor da jogada atual / "EMPATE" / ""
	VencedorRodada  string           `json:"vencedorRodada"`  // nome do vencedor da rodada / "EMPATE" / ""
	NumeroRodada    int              `json:"numeroRodada"`    // Número da rodada atual (1, 2, 3...)
	PontosRodada    map[string]int   `json:"pontosRodada"`    // nome -> pontos na rodada atual
	PontosPartida   map[string]int   `json:"pontosPartida"`   // nome -> rodadas ganhas na partida
}

// BAREMA ITEM 3: API REMOTA - Notificação de fim de partida
// Enviada quando a partida termina, indicando o vencedor final
type DadosFimDeJogo struct {
	VencedorNome string `json:"vencedorNome"` // Nome do vencedor final / "EMPATE" em caso de empate
}

/* ===================== Erro ===================== */

// BAREMA ITEM 4: ENCAPSULAMENTO - Estrutura para mensagens de erro
// Usada para comunicar erros de validação, operações inválidas, etc.
type DadosErro struct {
	Mensagem string `json:"mensagem"` // Descrição do erro ocorrido
}

/* ===================== Ping ===================== */

// BAREMA ITEM 6: LATÊNCIA - Estrutura para medição de latência
// O cliente envia um timestamp e o servidor responde com o mesmo timestamp
type DadosPing struct {
	Timestamp int64 `json:"timestamp"` // Timestamp em milissegundos para cálculo de latência
}

// BAREMA ITEM 6: LATÊNCIA - Resposta do servidor para medição de latência
// O servidor ecoa o timestamp recebido para permitir cálculo da latência
type DadosPong struct {
	Timestamp int64 `json:"timestamp"` // Timestamp ecoado do ping original
}
