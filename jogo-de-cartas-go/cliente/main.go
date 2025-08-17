package main

import(
	"fmt"
	"net"
	"strconv"
	"time"
)

func main(){
	endereco := "servidor:65432"

	conn, err := net.Dial("tcp", endereco)
	if err != nil{
		fmt.Printf("Não foi possível conectar ao servidor: %s\n", err)
		return
	}
	defer conn.Close()
	fmt.Printf("Conectado ao servidor em %s\n", endereco)

	tempoInicial := time.Now()

	mensagem := strconv.FormatInt(tempoinicial.UnixNano(), 10)

	
}