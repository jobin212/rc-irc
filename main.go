package main

import (
	"bufio"
	"flag"
	"fmt"
	"log"
	"net"
	"strings"
	"sync"
	"time"
)

const (
	VERSION  = "1.0.0"
	layoutUS = "January 2, 2006"
)

// TODO - handle improperly closed connections cleanly

var (
	port             = flag.String("p", "8080", "http service address")
	operatorPassword = flag.String("o", "pw", "operator password")
	nickToConn       = map[string]*IRCConn{}
	nameToChan       = map[string]*IRCChan{}
	nickToConnMtx    = sync.Mutex{}
	nameToChanMtx    = sync.Mutex{}
	connsMtx         = sync.Mutex{}
	chansMtx         = sync.Mutex{}
	ircConns         = []*IRCConn{}
	ircChans         = []*IRCChan{}
	timeCreated      = time.Now().Format(layoutUS)
	commandMap       = map[string]*IRCCommand{
		"NICK": {
			handler:          handleNick,
			minParams:        1,
			disableAutoReply: false,
			welcomeRequired:  false, // does the user have to be registered before trying to run this command
		},
		"USER": {
			handler:          handleUser,
			minParams:        4,
			disableAutoReply: false,
			welcomeRequired:  false,
		},
		"QUIT": {
			handler:          handleQuit,
			minParams:        0,
			disableAutoReply: false,
			welcomeRequired:  true,
		},
		"PRIVMSG": {
			handler:          handlePrivMsg,
			minParams:        2,
			disableAutoReply: false,
			welcomeRequired:  true,
		},
		"PING": {
			handler:          handlePing,
			minParams:        0,
			disableAutoReply: false,
			welcomeRequired:  true,
		},
		"PONG": {
			handler:          handlePong,
			minParams:        0,
			disableAutoReply: true,
			welcomeRequired:  true,
		},
		"MOTD": {
			handler:          handleMotd,
			minParams:        0,
			disableAutoReply: false,
			welcomeRequired:  true,
		},
		"NOTICE": {
			handler:          handleNotice,
			minParams:        2,
			disableAutoReply: true,
			welcomeRequired:  true,
		},
		"WHOIS": {
			handler:          handleWhoIs,
			minParams:        0,
			disableAutoReply: false,
			welcomeRequired:  true,
		},
		"LUSERS": {
			handler:          handleLUsers,
			minParams:        0,
			disableAutoReply: false,
			welcomeRequired:  true,
		},
		"JOIN": {
			handler:          handleJoin,
			minParams:        1,
			disableAutoReply: false,
			welcomeRequired:  true,
		},
		"PART": {
			handler:          handlePart,
			minParams:        1,
			disableAutoReply: false,
			welcomeRequired:  true,
		},
		"TOPIC": {
			handler:          handleTopic,
			minParams:        1,
			disableAutoReply: false,
			welcomeRequired:  true,
		},
	}
)

type IRCConn struct {
	User     string
	Nick     string
	Conn     net.Conn
	RealName string
	Welcomed bool
}

type IRCChan struct {
	Mtx     sync.Mutex
	Name    string
	Topic   string
	OpNicks map[string]bool
	Members []*IRCConn
}

type IRCCommand struct {
	minParams        int
	handler          func(ic *IRCConn, params string)
	disableAutoReply bool
	welcomeRequired  bool
}

func main() {
	flag.Parse()

	listener, err := net.Listen("tcp", fmt.Sprintf(":%s", *port))
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

			ircConn := &IRCConn{Conn: conn, Nick: "*"}
			connsMtx.Lock()
			ircConns = append(ircConns, ircConn)
			connsMtx.Unlock()

			go handleConnection(ircConn)
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
		if len(incoming_message) >= 510 {
			incoming_message = incoming_message[:510]
		}

		incoming_message = strings.Trim(incoming_message, " ")
		log.Printf("Incoming message: %s", incoming_message)
		split_message := strings.SplitN(incoming_message, " ", 2)

		if len(split_message) == 0 {
			continue
		}

		var prefix string = ""
		if strings.HasPrefix(split_message[0], ":") {
			prefix = split_message[0]
			log.Printf("Prefix %s\n", prefix)
			split_message = strings.SplitN(strings.Trim(split_message[1], " "), " ", 2)
		}

		command := split_message[0]

		var params string = ""
		if len(split_message) >= 2 {
			params = strings.Trim(split_message[1], " ")
		}

		ircCommand, ok := commandMap[command]
		if !ok {
			log.Println("not ok")
			handleDefault(ic, params, command)
			continue
		}

		if !validateWelcome(*ircCommand, ic) {
			continue
		}

		ircCommand.handler(ic, params)
	}
	err := scanner.Err()
	if err != nil {
		log.Printf("ERR: %v\n", err)
	}
}

func validateWelcome(command IRCCommand, ic *IRCConn) bool {
	if command.welcomeRequired && !ic.Welcomed {
		if command.disableAutoReply {
			return false
		}

		msg := fmt.Sprintf(
			":%s 451 %s :You have not registered\r\n",
			ic.Conn.LocalAddr(), ic.Nick)

		log.Printf(msg)
		_, err := ic.Conn.Write([]byte(msg))
		if err != nil {
			log.Fatal(err)
		}
		return false
	}

	return true
}

func validateParameters(command, params string, expectedNumParams int, ic *IRCConn) bool {
	paramVals := strings.Fields(params)
	if len(paramVals) >= expectedNumParams {
		return true
	}

	var msg string
	if command == "NICK" {
		msg = fmt.Sprintf(
			":%s 431 %s :No nickname given\r\n",
			ic.Conn.LocalAddr(), ic.Nick)
	} else if command == "PRIVMSG" && len(paramVals) == 0 {
		msg = fmt.Sprintf(
			":%s 411 %s :No recipient given (%s)\r\n",
			ic.Conn.LocalAddr(), ic.Nick, command)
	} else if command == "PRIVMSG" && len(paramVals) == 1 {
		msg = fmt.Sprintf(
			":%s 412 %s :No text to send\r\n",
			ic.Conn.LocalAddr(), ic.Nick)
	} else {
		msg = fmt.Sprintf(
			":%s 461 %s %s :Not enough parameters\r\n",
			ic.Conn.LocalAddr(), ic.Nick, command)
	}

	log.Printf(msg)
	_, err := ic.Conn.Write([]byte(msg))
	if err != nil {
		log.Fatal(err)
	}
	return false
}

func removePrefix(s string) string {
	split := strings.SplitN(s, ":", 2)
	if len(split) == 1 {
		return split[0]
	} else {
		return split[1]
	}
}
