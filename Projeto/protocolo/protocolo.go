package protocolo

import "encoding/json"

// Envelope base
type Mensagem struct {
	Comando string          `json:"comando"`
	Dados   json.RawMessage `json:"dados"`
}

/* ===================== Cartas / Inventário ===================== */

// Carta é única no estoque (id) e possui raridade.
// Campos extras (Naipe/Nome/Valor) ajudam a exibir le jogar.
type Carta struct {
	ID       string `json:"id"`
	Nome     string `json:"nome"`
	Naipe    string `json:"naipe"`              // "♠", "♥", "♦", "♣"
	Valor    int    `json:"valor"`              // 1..13 (Ás=1, Rei=13)
	Raridade string `json:"raridade,omitempty"` // C, U, R, L
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

type DadosJogarCarta struct {
	CartaID string `json:"cartaID"`
}

type DadosReceberChat struct {
	NomeJogador string `json:"nomeJogador"`
	Texto       string `json:"texto"`
}

/* ===================== Atualizações de jogo ===================== */

type DadosAtualizacaoJogo struct {
	MensagemDoTurno string           `json:"mensagemDoTurno"`
	ContagemCartas  map[string]int   `json:"contagemCartas"` // nome -> restantes no inventário
	UltimaJogada    map[string]Carta `json:"ultimaJogada"`   // nome -> carta recém jogada
	VencedorJogada  string           `json:"vencedorJogada"` // nome / EMPATE / ""
	VencedorRodada  string           `json:"vencedorRodada"` // nome / EMPATE / ""
	NumeroRodada    int              `json:"numeroRodada"`   // 1, 2, 3
	PontosRodada    map[string]int   `json:"pontosRodada"`   // nome -> pontos na rodada atual
	PontosPartida   map[string]int   `json:"pontosPartida"`  // nome -> rodadas ganhas
}

type DadosFimDeJogo struct {
	VencedorNome string `json:"vencedorNome"` // "EMPATE" em caso de empate
}

/* ===================== Erro ===================== */

type DadosErro struct {
	Mensagem string `json:"mensagem"`
}

/* ===================== Ping ===================== */

type DadosPing struct {
	Timestamp int64 `json:"timestamp"`
}

type DadosPong struct {
	Timestamp int64 `json:"timestamp"`
}
