package main

import (
	"log"

	"github.com/knowledgemeshgrid/knowledgemesh/internal/relay"
)

func main() {
	if err := relay.NewServeCommand().Execute(); err != nil {
		log.Fatal(err)
	}
}

