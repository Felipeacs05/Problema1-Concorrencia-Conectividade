// Projeto/protocolo/protocolo.go
package protocolo

import "encoding/json"

type Mensagem struct {
	Comando string          `json:"comando"`
	Dados   json.RawMessage `json:"dados"`
}

// Carta representa uma única carta do baralho.
type Carta struct {
	Nome  string `json:"nome"`  // Ex: "Carta de Poder 8"
	Valor int    `json:"valor"` // Poder de 1 a 10
}

// --- STRUCTS DO CLIENTE PARA O SERVIDOR ---

type DadosLogin struct {
	Nome string `json:"nome"`
}

type DadosEnviarChat struct {
	Texto string `json:"texto"`
}

type DadosComprarPacote struct {
	// Não precisa de dados, o comando em si é a ação.
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

type DadosAtualizacaoJogo struct {
	MensagemDoTurno string           `json:"mensagemDoTurno"`
	ContagemCartas  map[string]int   `json:"contagemCartas"`
	UltimaJogada    map[string]Carta `json:"ultimaJogada"`
	VencedorRodada  string           `json:"vencedorRodada"`
}

type DadosFimDeJogo struct {
	VencedorNome string `json:"vencedorNome"`
}

type DadosPacoteComprado struct {
	CartasRecebidas []Carta `json:"cartasRecebidas"`
	EstoqueRestante int     `json:"estoqueRestante"`
}

type DadosErro struct {
	Mensagem string `json:"mensagem"`
}