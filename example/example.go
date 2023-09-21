package main

import (
	"io"
	"log"
	"os"

	"github.com/PlakarLabs/go-ringbuffer"
)

func main() {
	if len(os.Args) != 2 {
		log.Fatal("a parameter is required")
	}

	rd, err := os.Open(os.Args[1])
	if err != nil {
		log.Fatal(err)
	}

	rb := ringbuffer.NewReaderSize(rd, 1024)
	io.Copy(os.Stdout, rb)
}
