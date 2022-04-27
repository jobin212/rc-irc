package main

import (
	"bufio"
	"fmt"
	"log"
	"net"
	"os"
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

	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		line := scanner.Text()
		irc_line := fmt.Sprintf("%s\r\n", line)
		_, err = conn.Write([]byte(irc_line))
		if err != nil {
			log.Fatal(err)
		}
	}

	err = scanner.Err()
	if err != nil {
		log.Printf("ERR: %v\n", err)
	}

	<-done
}
