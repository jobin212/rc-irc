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
		"AWAY": {
			handler:          handleAway,
			minParams:        0,
			disableAutoReply: false,
			welcomeRequired:  true,
		},
		"LIST": {
			handler:          handleList,
			minParams:        0,
			disableAutoReply: false,
			welcomeRequired:  true,
		},
	}
	replyMap = map[string]*IRCReply{
		"ERR_NOSUCHNICK": {
			NumParams:    1,
			Code:         401,
			FormatText:   "%s :No such nick/channel",
			UseGenerator: true,
			Generator: func(r *IRCReply, p []string) string {
				return fmt.Sprintf(r.FormatText, p[0])
			},
		},
		"ERR_CANNOTSENDTOCHAN": {
			NumParams:    1,
			Code:         404,
			FormatText:   "%s :Cannot send to channel",
			UseGenerator: true,
			Generator: func(r *IRCReply, p []string) string {
				return fmt.Sprintf(r.FormatText, p[0])
			},
		},
		"ERR_NORECIPIENT": {
			NumParams:    1,
			Code:         411,
			FormatText:   ":No recipient given (%s)",
			UseGenerator: true,
			Generator: func(r *IRCReply, p []string) string {
				return fmt.Sprintf(r.FormatText, p[0])
			},
		},
		"ERR_NOTEXTTOSEND": {
			NumParams:    0,
			Code:         412,
			FormatText:   ":No text to send",
			UseGenerator: false,
			Generator:    nil,
		},
		"ERR_NOMOTD": {
			NumParams:    0,
			Code:         422,
			FormatText:   ":MOTD File is missing",
			UseGenerator: false,
		},
		"ERR_NONICKNAMEGIVEN": {
			NumParams:    0,
			Code:         431,
			FormatText:   ":No nickname given",
			UseGenerator: false,
			Generator:    nil,
		},
		"ERR_NOTONCHANNEL": {
			NumParams:    1,
			Code:         442,
			FormatText:   "%s :You're not on that channel",
			UseGenerator: true,
			Generator: func(r *IRCReply, p []string) string {
				return fmt.Sprintf(r.FormatText, p[0])
			},
		},
		"ERR_NOTREGISTERED": {
			NumParams:    0,
			Code:         451,
			FormatText:   ":You have not registered",
			UseGenerator: false,
		},
		"ERR_NEEDMOREPARAMS": {
			NumParams:    1,
			Code:         461,
			FormatText:   "%s :Not enough parameters",
			UseGenerator: true,
			Generator: func(r *IRCReply, p []string) string {
				return fmt.Sprintf(r.FormatText, p[0])
			},
		},
	}
)

type IRCConn struct {
	User        string
	Nick        string
	Conn        net.Conn
	RealName    string
	Welcomed    bool
	AwayMessage string
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
	handler          func(ic *IRCConn, im IRCMessage)
	disableAutoReply bool
	welcomeRequired  bool
}

type IRCReply struct {
	NumParams    int
	Code         int
	FormatText   string
	UseGenerator bool
	Generator    func(r *IRCReply, p []string) string
}

type IRCMessage struct {
	Prefix  string
	Command string
	Params  []string
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

func extractMessage(rawMsg []byte) (IRCMessage, error) {
	var im IRCMessage
	msgStr := string(rawMsg)
	prefix := ""
	if msgStr[0] == ':' {
		// Message has a prefix
		splitMsg := strings.SplitN(msgStr, " ", 2)
		prefix = splitMsg[0]
		msgStr = splitMsg[1]
	}
	// Split off trailing if present
	msgParts := strings.Split(msgStr, ":")
	trailing := ""
	msgStr = msgParts[0]
	if len(msgParts) == 2 {
		// msg has `trailing` param
		trailing = msgParts[1]
	} else if len(msgParts) != 1 {
		// More than one instance of ':' in command + params
		// this should not occur and indicates a malformed message
		return im, fmt.Errorf("in extractMessage, multiple colons found")
	}
	commandAndParams := strings.Split(strings.Trim(msgStr, " "), " ")
	command := commandAndParams[0]
	var params = make([]string, 0, 15)
	if len(commandAndParams) != 1 {
		for _, v := range commandAndParams[1:] {
			if v != "" && v != " " {
				params = append(params, v)
			}
		}
	}
	if trailing != "" {
		params = append(params, trailing)
	}
	im = IRCMessage{
		Prefix:  prefix,
		Command: command,
		Params:  params,
	}
	return im, nil
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

		im, err := extractMessage([]byte(incoming_message))

		if err != nil {
			log.Printf("Error extracting message %v", err)
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

		ircCommand, ok := commandMap[command]
		if !ok {
			log.Println("not ok")
			//handleDefault(ic, params, command)
			handleDefault(ic, im)
			continue
		}

		if !validateWelcome(*ircCommand, ic) {
			continue
		}

		//ircCommand.handler(ic, params)
		ircCommand.handler(ic, im)
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

		rpl := replyMap["ERR_NOTREGISTERED"]
		msg, _ := formatReply(ic, rpl, []string{})

		log.Printf(msg)
		_, err := ic.Conn.Write([]byte(msg))
		if err != nil {
			log.Fatal(err)
		}
		return false
	}

	return true
}

func formatReply(ic *IRCConn, r *IRCReply, p []string) (string, error) {

	if len(p) != r.NumParams {
		return "", fmt.Errorf("in sendReply - param number mismatch - expected: %d received %d",
			r.NumParams, len(p))
	}

	var generated string
	if r.UseGenerator {
		generated = r.Generator(r, p)
	} else {
		generated = r.FormatText
	}

	partiallyFormatted := fmt.Sprintf(":%s %d %s %s\r\n",
		ic.Conn.LocalAddr(), r.Code, ic.Nick, generated)
	return partiallyFormatted, nil
}

func validateParameters(command, params string, expectedNumParams int, ic *IRCConn) bool {
	paramVals := strings.Fields(params)
	if len(paramVals) >= expectedNumParams {
		return true
	}

	var msg string
	if command == "NICK" {
		rpl := replyMap["ERR_NONICKNAMEGIVEN"]
		msg, _ = formatReply(ic, rpl, []string{})
	} else if command == "PRIVMSG" && len(paramVals) == 0 {
		rpl := replyMap["ERR_NORECIPIENT"]
		msg, _ = formatReply(ic, rpl, []string{command})
	} else if command == "PRIVMSG" && len(paramVals) == 1 {
		rpl := replyMap["ERR_NOTEXTTOSEND"]
		msg, _ = formatReply(ic, rpl, []string{})
	} else {
		rpl := replyMap["ERR_NEEDMOREPARAMS"]
		msg, _ = formatReply(ic, rpl, []string{command})
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
