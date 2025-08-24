package protocolo

import "encoding/json"

type Mensagem struct {
	Comando string  `json:"comando"`
	Dados   json.RawMessage `json:"dados"`
}

type DadosLogin struct {
	Nome string `json:"nome"`
}

type DadosErro struct {
	Mensagem string `json:"mensagem"`
}

type DadosCriarSala struct {
	NomeDaSala string `json:"nomeDaSala"`
}

type DadosSalaCriada struct {
	SalaID string `json:"salaID"`
}

type DadosEnviarChat struct {
	Texto string `json:"texto"`
}

type DadosReceberChat struct {
	NomeJogador string `json:"nomeJogador"`
	Texto       string `json:"texto"`
}