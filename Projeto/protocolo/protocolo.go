package protocolo

type Mensagem struct {
	Comando string `json:"comando"`
	Dados interface{} `json:"dados"`
}

type DadosLogin struct {
	Nome string `json:"nome"`
	Senha string `json:"senha"`
	Id string `json:"id"`
}

type DadosErro struct {
	Mensagem string `json:"mensagem"`
}

type DadosCriarSala struct {
	NomeDaSala string `json:"nomeDaSala"`
}

type DadosSalaCriada struct{
	SalaID string `json:"salaID"`
}

type DadosEnviarChat struct{
	Texto string `json:"mensagem"`
}

type DadosReceberChat struct{
	Texto string `json:"mensagem"`
	NomeJogadorEnviou string `json:"NomeJogadorEnviou"`
}