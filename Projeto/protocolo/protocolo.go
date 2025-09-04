// Projeto/protocolo/protocolo.go
package protocolo

import "encoding/json"

type Mensagem struct {
	Comando string          `json:"comando"`
	Dados   json.RawMessage `json:"dados"`
}

// Carta representa uma única carta do baralho.
type Carta struct {
	Naipe  string `json:"naipe"`
	Valor  int    `json:"valor"` // 2-10, Valete=11, Dama=12, Rei=13, Ás=14
	Nome   string `json:"nome"`  // Ex: "Ás de Espadas"
}

// --- STRUCTS DO CLIENTE PARA O SERVIDOR ---

type DadosLogin struct {
	Nome string `json:"nome"`
}

type DadosEnviarChat struct {
	Texto string `json:"texto"`
}

// --- STRUCTS DO SERVIDOR PARA O CLIENTE ---

type DadosPartidaEncontrada struct {
	SalaID       string `json:"salaID"`
	OponenteNome string `json:"oponenteNome"`
}

type DadosReceberChat struct {
	NomeJogador string `json:"nomeJogador"`
	Texto       string `json:"texto"`
}

// DadosAtualizacaoJogo envia o estado completo do jogo para os clientes.
type DadosAtualizacaoJogo struct {
	MensagemDoTurno string           `json:"mensagemDoTurno"`
	ContagemCartas  map[string]int   `json:"contagemCartas"`
	UltimaJogada    map[string]Carta `json:"ultimaJogada"`
	VencedorRodada  string           `json:"vencedorRodada"`
}

// DadosFimDeJogo informa o fim da partida e quem foi o vencedor.
type DadosFimDeJogo struct {
	VencedorNome string `json:"vencedorNome"`
}