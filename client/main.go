package main

import (
	"bufio"
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

	done := make(chan struct{})

	go func(c net.Conn) {
		defer func() {
			c.Close()
			done <- struct{}{}
		}()

		scanner := bufio.NewScanner(c)
		scanner.Split(bufio.ScanLines)

		for scanner.Scan() {
			log.Println(scanner.Text())
		}

		err = scanner.Err()
		if err != nil {
			log.Printf("ERR: %v\n", err)
		}
	}(conn)

	_, err = conn.Write([]byte("NICK jobin\r\n"))
	if err != nil {
		log.Fatal(err)
	}

	// _, err = conn.Write([]byte("NICK jobin\r\n"))
	// if err != nil {
	// 	log.Fatal(err)
	// }

	_, err = conn.Write([]byte("USER jobin * * :jobin212\r\n"))
	if err != nil {
		log.Fatal(err)
	}

	<-done
}
