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

	scanner := bufio.NewScanner(ic.Conn)
	scanner.Split(bufio.ScanLines)

	for scanner.Scan() {
		incoming_message := scanner.Text()
		log.Printf("Incoming message: %s", incoming_message)
		split_message := strings.SplitN(incoming_message, " ", 2)

		if len(split_message) == 0 {
			continue
		}

		var prefix string = ""
		if strings.HasPrefix(split_message[0], ":") {
			prefix = split_message[0]
			log.Printf("Prefix %s\n", prefix)
			split_message = strings.SplitN(split_message[1], " ", 2)
		}

		command := split_message[0]

		var params string = ""
		if len(split_message) >= 2 {
			params = split_message[1]
		}

		switch command {
		case "NICK":
			handleNick(ic, params)
		case "USER":
			handleUser(ic, params)
		case "QUIT":
			handleQuit(ic, params)
		default:
			log.Println("Command not recognized")
		}

	}
	err := scanner.Err()
	if err != nil {
		log.Printf("ERR: %v\n", err)
	}
}

func handleQuit(ic *IRCConn, params string) {
	if !validateWelcomeAndParameters("QUIT", params, 0, ic) {
		return
	}

	quitMessage := "Client Quit"
	if params != "" {
		quitMessage = params
	}

	msg := fmt.Sprintf("Closing Link: %s %s", ic.Conn.RemoteAddr(), quitMessage)
	_, err := ic.Conn.Write([]byte(msg))
	if err != nil {
		log.Fatal(err)
	}
	// TODO close gracefully, cleaup
	err = ic.Conn.Close()
	if err != nil {
		log.Println(err)
	}
}

func handleNick(ic *IRCConn, params string) {
	if !validateWelcomeAndParameters("NICK", params, 1, ic) {
		return
	}

	nick := strings.SplitN(params, " ", 2)[0]

	_, ok := nickInUse[nick]
	if nick != ic.Nick && ok {
		msg := fmt.Sprintf(":%s 433 * %s :Nickname is already in use\r\n",
			ic.Conn.LocalAddr(), nick)
		_, err := ic.Conn.Write([]byte(msg))
		if err != nil {
			log.Fatal(err)
		}
		return
	}

	nickInUse[nick] = true
	ic.Nick = nick

	checkAndSendWelcome(ic)
}

func handleUser(ic *IRCConn, params string) {
	if !validateWelcomeAndParameters("USER", params, 4, ic) {
		return
	}

	if ic.User != "" {
		msg := fmt.Sprintf(
			":%s 463 :You may not reregister\r\n",
			ic.Conn.LocalAddr())
		_, err := ic.Conn.Write([]byte(msg))
		if err != nil {
			log.Fatal(err)
		}

		return
	}

	ic.User = strings.SplitN(params, " ", 2)[0]

	checkAndSendWelcome(ic)
}

func checkAndSendWelcome(ic *IRCConn) {
	if !ic.Welcomed && ic.Nick != "" && ic.User != "" {
		msg := fmt.Sprintf(
			":%s 001 %s :Welcome to the Internet Relay Network %s!%s@%s\r\n",
			ic.Conn.LocalAddr(), ic.Nick, ic.Nick, ic.User, ic.Conn.RemoteAddr().String())

		log.Printf(msg)
		_, err := ic.Conn.Write([]byte(msg))
		if err != nil {
			log.Fatal(err)
		}
		ic.Welcomed = true
	}
}

func validateWelcomeAndParameters(command, params string, expectedNumParams int, ic *IRCConn) bool {
	if command != "NICK" && command != "USER" && !ic.Welcomed {
		msg := fmt.Sprintf(
			":%s 451 :You have not registered\r\n",
			ic.Conn.LocalAddr())

		log.Printf(msg)
		_, err := ic.Conn.Write([]byte(msg))
		if err != nil {
			log.Fatal(err)
		}
		return false
	}

	paramVals := strings.Fields(params)
	if len(paramVals) >= expectedNumParams {
		return true
	}

	var msg string
	if command == "NICK" {
		msg = fmt.Sprintf(
			":%s 431 :No nickname given\r\n",
			ic.Conn.LocalAddr())
	} else {
		msg = fmt.Sprintf(
			":%s 461 %s :Not enough parameters\r\n",
			ic.Conn.LocalAddr(), command)
	}

	log.Printf(msg)
	_, err := ic.Conn.Write([]byte(msg))
	if err != nil {
		log.Fatal(err)
	}
	return false
}
