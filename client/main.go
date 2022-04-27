package main

import (
	"log"
	"net"
)

func main() {
	addr := "127.0.0.1:8080"
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()
	log.Printf("Connected to %s\n", addr)

	_, err = conn.Write([]byte("NICK jobin\r\n"))
	if err != nil {
		log.Fatal(err)
	}

	_, err = conn.Write([]byte("USER jobin * * :jobin212\r\n"))
	if err != nil {
		log.Fatal(err)
	}
}
