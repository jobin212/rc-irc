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
			handler:     handleNick,
			minParams:   1,
			silentReply: false,
		},
		"USER": {
			handler:     handleUser,
			minParams:   4,
			silentReply: false,
		},
		"QUIT": {
			handler:     handleQuit,
			minParams:   0,
			silentReply: false,
		},
		"PRIVMSG": {
			handler:     handlePrivMsg,
			minParams:   2,
			silentReply: false,
		},
		"PING": {
			handler:     handlePing,
			minParams:   0,
			silentReply: false,
		},
		"PONG": {
			handler:     handlePong,
			minParams:   0,
			silentReply: false,
		},
		"MOTD": {
			handler:     handleMotd,
			minParams:   0,
			silentReply: false,
		},
		"NOTICE": {
			handler:     handleNotice,
			minParams:   2,
			silentReply: true,
		},
		"WHOIS": {
			handler:     handleWhoIs,
			minParams:   0,
			silentReply: false,
		},
		"LUSERS": {
			handler:     handleLUsers,
			minParams:   0,
			silentReply: false,
		},
		"JOIN": {
			handler:     handleJoin,
			minParams:   1,
			silentReply: false,
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
	minParams   int
	handler     func(ic *IRCConn, params string)
	silentReply bool
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
		} else {
			ircCommand.handler(ic, params)
		}
	}
	err := scanner.Err()
	if err != nil {
		log.Printf("ERR: %v\n", err)
	}
}

func removePrefix(s string) string {
	split := strings.SplitN(s, ":", 2)
	if len(split) == 1 {
		return split[0]
	} else {
		return split[1]
	}
}
