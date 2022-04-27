package main

import (
	"bufio"
	"flag"
	"fmt"
	"log"
	"net"
	"strings"
)

var (
	port             = flag.String("p", "7776", "http service address")
	operatorpassword = flag.String("o", "pw", "password")
	nickInUse        = map[string]bool{}
)

type IRCConn struct {
	User     string
	Nick     string
	Conn     net.Conn
	Welcomed bool
}

func main() {
	flag.Parse()

	listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%s", *port))
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

			go handleConnection(&IRCConn{Conn: conn})
		}
	}()

	<-done
}

func handleConnection(ic *IRCConn) {
	defer func() {
		ic.Conn.Close()
	}()

	client_address := ic.Conn.RemoteAddr().String()

	scanner := bufio.NewScanner(ic.Conn)
	scanner.Split(bufio.ScanLines)

	for scanner.Scan() {
		incoming_message := scanner.Text()
		log.Printf("Incoming message: %s", incoming_message)
		split_message := strings.Fields(incoming_message)

		if len(split_message) == 0 {
			continue
		}

		var prefix string
		if strings.HasPrefix(split_message[0], ":") {
			prefix = split_message[0]
			log.Printf("Prefix %s\n", prefix)
			split_message = split_message[1:]
		}

		command := split_message[0]
		params := split_message[1:]

		switch command {
		case "NICK":
			nick := params[0]
			log.Printf("NICK for connection %s is %s\n", client_address, nick)

			_, ok := nickInUse[nick]
			if nick != ic.Nick && ok {
				msg := fmt.Sprintf(":%s 433 * %s :Nickname is already in use.\r\n",
					ic.Conn.LocalAddr(), nick)
				log.Printf(msg)
				_, err := ic.Conn.Write([]byte(msg))
				if err != nil {
					log.Fatal(err)
				}
				break
			}

			nickInUse[nick] = true
			ic.Nick = nick
			if !ic.Welcomed && ic.User != "" {
				msg := fmt.Sprintf(
					":%s 001 %s :Welcome to the Internet Relay Network %s!%s@%s\r\n",
					ic.Conn.LocalAddr(), ic.Nick, ic.Nick, ic.User, client_address)

				log.Printf(msg)
				_, err := ic.Conn.Write([]byte(msg))
				if err != nil {
					log.Fatal(err)
				}
				ic.Welcomed = true
			}
		case "USER":
			username := params[0]
			ic.User = username

			if !ic.Welcomed && ic.Nick != "" {
				msg := fmt.Sprintf(
					":%s 001 %s :Welcome to the Internet Relay Network %s!%s@%s\r\n",
					ic.Conn.LocalAddr(), ic.Nick, ic.Nick, ic.User, client_address)

				log.Printf(msg)
				_, err := ic.Conn.Write([]byte(msg))
				if err != nil {
					log.Fatal(err)
				}
				ic.Welcomed = true
			}
		default:
			log.Println("Command not recognized")
		}

	}
	err := scanner.Err()
	if err != nil {
		log.Printf("ERR: %v\n", err)
	}
}
