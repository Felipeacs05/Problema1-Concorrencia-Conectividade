package protocolo

import "encoding/json"

// Envelope base
type Mensagem struct {
	Comando string          `json:"comando"`
	Dados   json.RawMessage `json:"dados"`
}

/* ===================== Cartas / Inventário ===================== */

// Carta é única no estoque (id) e possui raridade.
// Campos extras (Naipe/Nome/Valor) ajudam a exibir e jogar.
type Carta struct {
	ID        string `json:"id"`
	Nome      string `json:"nome"`
	Naipe     string `json:"naipe,omitempty"`
	Valor     int    `json:"valor"`               // 1..13 (maior vence)
	Raridade  string `json:"raridade,omitempty"`  // C, U, R, L
}

type ComprarPacoteReq struct {
	Quantidade int `json:"quantidade"` // opcional; 1 = 10 cartas. Se zero, servidor assume 1.
}

type ComprarPacoteResp struct {
	Cartas          []Carta `json:"cartas"`
	EstoqueRestante int     `json:"estoqueRestante"`
}

/* ===================== Login / Match / Chat ===================== */

type DadosLogin struct {
	Nome string `json:"nome"`
}

type DadosPartidaEncontrada struct {
	SalaID       string `json:"salaID"`
	OponenteNome string `json:"oponenteNome"`
}

type DadosEnviarChat struct {
	Texto string `json:"texto"`
}

type DadosReceberChat struct {
	NomeJogador string `json:"nomeJogador"`
	Texto       string `json:"texto"`
}

/* ===================== Atualizações de jogo ===================== */

type DadosAtualizacaoJogo struct {
	MensagemDoTurno string           `json:"mensagemDoTurno"`
	ContagemCartas  map[string]int   `json:"contagemCartas"`  // nome -> restantes no baralho
	UltimaJogada    map[string]Carta `json:"ultimaJogada"`    // nome -> carta recém jogada
	VencedorRodada  string           `json:"vencedorRodada"`  // nome / EMPATE / ""
}

type DadosFimDeJogo struct {
	VencedorNome string `json:"vencedorNome"` // "EMPATE" em caso de empate
}

/* ===================== Erro ===================== */

type DadosErro struct {
	Mensagem string `json:"mensagem"`
}
