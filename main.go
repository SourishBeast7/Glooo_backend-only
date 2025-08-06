package main

import (
	"fmt"

	g "github.com/SourishBeast7/Glooo/http-server"
	"github.com/joho/godotenv"
)

func main() {
	err := godotenv.Load(".env.local")
	if err != nil {
		fmt.Println(err.Error())
	}
	server := g.NewServer(":3000")
	server.HandleRoutes()
}
