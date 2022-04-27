package main

import (
	"bufio"
	"flag"
	"log"
	"net"
	"strings"
)

var (
	addr             = flag.String("addr", ":8080", "http service address")
	nickToConnection = map[string]string{}
)

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

			server_address := conn.LocalAddr().String()

			go func(c net.Conn) {
				defer func() {
					c.Close()
				}()

				client_address := c.RemoteAddr().String()

				scanner := bufio.NewScanner(c)
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
						nick := params[0]
						log.Printf("NICK for connection %s is %s\n", client_address, nick)

						val, ok := nickToConnection[nick]
						if !ok {
							nickToConnection[nick] = client_address
						} else {
							if val == client_address {
								log.Println("IGNORING REPEAT NICK")
							} else {
								log.Printf(":%s 433 * %s :Nickname is already in use.\r\n",
									server_address, nick)
							}
						}
					case "USER":
						log.Printf(
							":%s 001 %s :Welcome to the Internet Relay Network %s!%s@%s\r\n",
							server_address, params[0], params[0], params[0], client_address)
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
