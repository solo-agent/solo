package main

import (
	"fmt"
	"os"
	"github.com/solo-ai/solo/internal/auth"
)

func main() {
	token, err := auth.GenerateAccessToken(
		"88435f49-beca-4fb7-a521-28b85b9fb8dd",
		"agent@mazi.solo",
		"麻子",
	)
	if err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}
	fmt.Println(token)
}
