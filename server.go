package main

import (
	"bufio"
	"flag"
	"log"
	"net"
	"strings"
)

var addr = flag.String("addr", ":8080", "http service address")

func main() {
	flag.Parse()

	listener, err := net.Listen("tcp", "127.0.0.1:8080")
	if err != nil {
		log.Fatal(err)
	}
	defer listener.Close()
	log.Printf("Listening on %s\n", listener.Addr().String())

	done := make(chan struct{})

	go func() {
		defer func() { done <- struct{}{} }()
		for {
			conn, err := listener.Accept()
			if err != nil {
				log.Println(err)
				return
			}

			go func(c net.Conn) {
				defer func() {
					conn.Close()
				}()

				scanner := bufio.NewScanner(conn)
				scanner.Split(bufio.ScanLines)

				for scanner.Scan() {
					incoming_message := scanner.Text()
					log.Println(incoming_message)
					split_message := strings.Fields(incoming_message)

					if len(split_message) == 0 {
						continue
					}

					if strings.HasPrefix(split_message[0], ":") {
						log.Printf("Prefix %s\n", split_message[0])
						split_message = split_message[1:]
					}

					command := split_message[0]
					params := split_message[1:]

					switch command {
					case "NICK":
						log.Printf("NICK for connection %s is %s\n", conn.RemoteAddr(), params[0])
					case "USER":
						log.Printf(
							":%s 001 %s :Welcome to the Internet Relay Network %s!%s@%s\n",
							conn.LocalAddr(), params[0], params[0], params[0], conn.RemoteAddr())
					default:
						log.Println("Command not recognized")
					}

				}
				err = scanner.Err()
				if err != nil {
					log.Printf("ERR: %v\n", err)
				}
			}(conn)
		}
	}()

	<-done
}
