package main

import (
	"log"

	"github.com/knowledgemeshgrid/knowledgemesh/internal/control"
)

func main() {
	if err := control.NewCommand().Execute(); err != nil {
		log.Fatal(err)
	}
}
