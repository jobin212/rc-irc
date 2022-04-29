package main

import (
	"bufio"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
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

		switch command {
		case "NICK":
			handleNick(ic, params)
		case "USER":
			handleUser(ic, params)
		case "QUIT":
			handleQuit(ic, params)
		case "PRIVMSG":
			handlePrivMsg(ic, params)
		case "PING":
			handlePing(ic, params)
		case "PONG":
			break
		case "MOTD":
			handleMotd(ic, params)
		case "NOTICE":
			handleNotice(ic, params)
		case "WHOIS":
			handleWhoIs(ic, params)
		case "LUSERS":
			handleLUsers(ic, params)
		case "JOIN":
			handleJoin(ic, params)
		case "":
			break
		default:
			handleDefault(ic, params, command)
		}

	}
	err := scanner.Err()
	if err != nil {
		log.Printf("ERR: %v\n", err)
	}
}

func handleLUsers(ic *IRCConn, params string) {
	if !validateWelcomeAndParameters("LUSERS", params, 0, ic) {
		return
	}

	writeLUsers(ic)
}

func writeLUsers(ic *IRCConn) {
	numServers, numServices, numOperators, numChannels := 1, 0, 0, 0
	numUsers, numUnknownConnections, numClients := 0, 0, 0

	connsMtx.Lock()
	for _, conn := range ircConns {
		if conn.Welcomed {
			numUsers++
			numClients++
		} else if conn.Nick != "*" || conn.User != "" {
			numClients++
		} else {
			numUnknownConnections++
		}
	}
	connsMtx.Unlock()

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf(":%s 251 %s :There are %d users and %d services on %d servers\r\n",
		ic.Conn.LocalAddr(), ic.Nick, numUsers, numServices, numServers))

	sb.WriteString(fmt.Sprintf(":%s 252 %s %d :operator(s) online\r\n",
		ic.Conn.LocalAddr(), ic.Nick, numOperators))

	sb.WriteString(fmt.Sprintf(":%s 253 %s %d :unknown connection(s)\r\n",
		ic.Conn.LocalAddr(), ic.Nick, numUnknownConnections))

	sb.WriteString(fmt.Sprintf(":%s 254 %s %d :channels formed\r\n",
		ic.Conn.LocalAddr(), ic.Nick, numChannels))

	sb.WriteString(fmt.Sprintf(":%s 255 %s :I have %d clients and %d servers\r\n",
		ic.Conn.LocalAddr(), ic.Nick, numClients, numServers))

	_, err := ic.Conn.Write([]byte(sb.String()))
	if err != nil {
		log.Fatal(err)
	}
}

func handleWhoIs(ic *IRCConn, params string) {
	if !ic.Welcomed {
		return
	}

	targetNick := strings.Trim(params, " ")

	if targetNick == "" {
		return
	}

	targetIc, ok := lookupNickConn(targetNick)

	if !ok {
		msg := fmt.Sprintf(":%s 401 %s %s :No such nick/channel\r\n", ic.Conn.LocalAddr(), ic.Nick, targetNick)
		_, err := ic.Conn.Write([]byte(msg))
		if err != nil {
			log.Println("error sending nosuchnick reply")
		}
		return
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf(":%s 311 %s %s %s %s * :%s\r\n",
		ic.Conn.LocalAddr(), ic.Nick, targetIc.Nick, targetIc.User, targetIc.Conn.RemoteAddr().String(), targetIc.RealName))

	sb.WriteString(fmt.Sprintf(":%s 312 %s %s %s :%s\r\n",
		ic.Conn.LocalAddr(), ic.Nick, targetIc.Nick, targetIc.Conn.LocalAddr().String(), "<server info>"))

	sb.WriteString(fmt.Sprintf(":%s 318 %s %s :End of WHOIS list\r\n",
		ic.Conn.LocalAddr(), ic.Nick, targetIc.Nick))

	_, err := ic.Conn.Write([]byte(sb.String()))
	if err != nil {
		log.Fatal(err)
	}

	return
}

func handleDefault(ic *IRCConn, params, command string) {
	if !ic.Welcomed {
		return
	}
	msg := fmt.Sprintf(":%s 421 %s %s :Unknown command\r\n",
		ic.Conn.LocalAddr(), ic.Nick, command)
	_, err := ic.Conn.Write([]byte(msg))
	if err != nil {
		log.Println("error sending nosuchnick reply")
	}
}

// Helper function to deal with the fact that nickToConn should be threadsafe
func lookupNickConn(nick string) (*IRCConn, bool) {
	nickToConnMtx.Lock()
	recipientIc, ok := nickToConn[nick]
	nickToConnMtx.Unlock()
	return recipientIc, ok
}

func handleNotice(ic *IRCConn, params string) {
	if !ic.Welcomed {
		return
	}

	splitParams := strings.SplitN(params, " ", 2)
	if len(splitParams) < 2 {
		return
	}
	targetNick, userMessage := splitParams[0], splitParams[1]

	// get connection from targetNick
	recipientIc, ok := lookupNickConn(targetNick)

	if !ok {
		return
	}

	msg := fmt.Sprintf(
		":%s!%s@%s NOTICE %s %s\r\n",
		ic.Nick, ic.User, ic.Conn.RemoteAddr(), targetNick, userMessage)
	_, err := recipientIc.Conn.Write([]byte(msg))
	if err != nil {
		log.Fatal(err)
	}

}

func handleMotd(ic *IRCConn, params string) {
	writeMotd(ic)
}

func writeMotd(ic *IRCConn) {
	dat, err := os.ReadFile("./motd.txt")
	if err != nil {
		msg := fmt.Sprintf(":%s 422 %s :MOTD File is missing\r\n",
			ic.Conn.LocalAddr(), ic.Nick)
		_, err := ic.Conn.Write([]byte(msg))
		if err != nil {
			log.Fatal(err)
		}
		return
	}
	motd := string(dat)

	motdLines := strings.FieldsFunc(motd, func(c rune) bool { return c == '\n' || c == '\r' })

	var sb strings.Builder

	sb.WriteString(fmt.Sprintf(":%s 375 %s :- %s Message of the day - \r\n",
		ic.Conn.LocalAddr(), ic.Nick, ic.Conn.LocalAddr()))

	for _, motdLine := range motdLines {
		log.Println(motdLine)
		sb.WriteString(fmt.Sprintf(":%s 372 %s :- %s\r\n",
			ic.Conn.LocalAddr(), ic.Nick, string(motdLine)))
	}

	sb.WriteString(fmt.Sprintf(":%s 376 %s :End of MOTD command\r\n",
		ic.Conn.LocalAddr(), ic.Nick))

	_, err = ic.Conn.Write([]byte(sb.String()))
	if err != nil {
		log.Fatal(err)
	}
}

func handlePing(ic *IRCConn, params string) {
	// TODO validate welcome?
	// TODO update ping to update connection lifetime?
	msg := fmt.Sprintf("PONG %s\r\n", ic.Conn.LocalAddr().String())
	_, err := ic.Conn.Write([]byte(msg))
	if err != nil {
		log.Fatal(err)
	}
}

func handlePrivMsg(ic *IRCConn, params string) {
	if !validateWelcomeAndParameters("PRIVMSG", params, 2, ic) {
		return
	}

	splitParams := strings.SplitN(params, " ", 2)
	targetNick, userMessage := strings.Trim(splitParams[0], " "), splitParams[1]

	// get connection from targetNick
	recipientIc, ok := lookupNickConn(targetNick)

	if !ok {
		msg := fmt.Sprintf(":%s 401 %s %s :No such nick/channel\r\n", ic.Conn.LocalAddr(), ic.Nick, targetNick)
		_, err := ic.Conn.Write([]byte(msg))
		if err != nil {
			log.Println("error sending nosuchnick reply")
		}
		return
	}

	msg := fmt.Sprintf(
		":%s!%s@%s PRIVMSG %s %s\r\n",
		ic.Nick, ic.User, ic.Conn.RemoteAddr(), targetNick, userMessage)
	if len(msg) > 512 {
		msg = msg[:510] + "\r\n"
	}
	_, err := recipientIc.Conn.Write([]byte(msg))
	if err != nil {
		log.Fatal(err)
	}
}

func handleQuit(ic *IRCConn, params string) {
	if !validateWelcomeAndParameters("QUIT", params, 0, ic) {
		return
	}

	quitMessage := "Client Quit"
	if params != "" {
		quitMessage = removePrefix(params)
	}

	msg := fmt.Sprintf("ERROR :Closing Link: %s (%s)\r\n", ic.Conn.RemoteAddr(), quitMessage)
	_, err := ic.Conn.Write([]byte(msg))
	if err != nil {
		log.Fatal(err)
	}

	nickToConnMtx.Lock()
	delete(nickToConn, ic.Nick)
	nickToConnMtx.Unlock()

	connsMtx.Lock()
	for idx, conn := range ircConns {
		if conn == ic {
			ircConns = append(ircConns[:idx], ircConns[idx+1:]...)
			break
		}
	}
	connsMtx.Unlock()

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

	prevNick := ic.Nick
	nick := strings.SplitN(params, " ", 2)[0]

	_, nickInUse := lookupNickConn(nick)
	if nick != ic.Nick && nickInUse { // TODO what happens if they try to change their own nick?
		msg := fmt.Sprintf(":%s 433 * %s :Nickname is already in use\r\n",
			ic.Conn.LocalAddr(), nick)
		_, err := ic.Conn.Write([]byte(msg))
		if err != nil {
			log.Fatal(err)
		}
		return
	}

	// if Nick has already been set
	nickToConnMtx.Lock()
	if prevNick != "*" {
		delete(nickToConn, prevNick)
	}

	nickToConn[nick] = ic
	nickToConnMtx.Unlock()
	ic.Nick = nick

	checkAndSendWelcome(ic)
}

func handleUser(ic *IRCConn, params string) {
	if !validateWelcomeAndParameters("USER", params, 4, ic) {
		return
	}

	if ic.Welcomed && ic.User != "" {
		msg := fmt.Sprintf(
			":%s 463 :You may not reregister\r\n",
			ic.Conn.LocalAddr())
		_, err := ic.Conn.Write([]byte(msg))
		if err != nil {
			log.Fatal(err)
		}

		return
	}

	splitParams := strings.SplitN(params, " ", 2)
	ic.User = splitParams[0]
	splitOnColon := strings.SplitN(splitParams[1], ":", 2)
	if len(splitOnColon) > 1 {
		ic.RealName = splitOnColon[1]
	} else {
		ic.RealName = strings.SplitN(strings.Trim(splitParams[1], " "), " ", 3)[2]
	}

	checkAndSendWelcome(ic)
}

func lookupChannelByName(name string) (*IRCChan, bool) {
	nameToChanMtx.Lock()
	ircCh, ok := nameToChan[name]
	nameToChanMtx.Unlock()
	return ircCh, ok
}

func addUserToChannel(ic *IRCConn, ircCh *IRCChan) {
	ircCh.Mtx.Lock()
	ircCh.Members = append(ircCh.Members, ic)
	lm := len(ircCh.Members)
	var members = make([]*IRCConn, lm, lm)
	copy(members, ircCh.Members)
	ircCh.Mtx.Unlock()
	joinMsg := fmt.Sprintf(":%s!%s@%s JOIN %s\r\n", ic.Nick, ic.User, ic.Conn.RemoteAddr(), ircCh.Name)
	for _, v := range members {
		v := v
		go func() {
			_, err := v.Conn.Write([]byte(joinMsg))

			if err != nil {
				log.Fatal(err)
			}
		}()
	}
}

func newChannel(ic *IRCConn, chanName string) *IRCChan {
	newChan := IRCChan{
		Mtx:     sync.Mutex{},
		Name:    chanName,
		Topic:   "",
		OpNicks: make(map[string]bool),
		Members: []*IRCConn{},
	}
	newChan.OpNicks[ic.Nick] = true
	chansMtx.Lock()
	ircChans = append(ircChans, &newChan)
	chansMtx.Unlock()

	nameToChanMtx.Lock()
	nameToChan[chanName] = &newChan
	nameToChanMtx.Unlock()
	return &newChan
}

func handleJoin(ic *IRCConn, params string) {
	chanName := params

	ircCh, ok := lookupChannelByName(chanName)
	if !ok {
		// Create new channel
		ircCh = newChannel(ic, chanName)
	}
	// Join channel
	addUserToChannel(ic, ircCh)
	// TODO need to send other replies here
}

func checkAndSendWelcome(ic *IRCConn) {
	if !ic.Welcomed && ic.Nick != "*" && ic.User != "" {
		msg := fmt.Sprintf(
			":%s 001 %s :Welcome to the Internet Relay Network %s!%s@%s\r\n",
			ic.Conn.LocalAddr(), ic.Nick, ic.Nick, ic.User, ic.Conn.RemoteAddr().String())

		log.Printf(msg)
		_, err := ic.Conn.Write([]byte(msg))
		if err != nil {
			log.Fatal(err)
		}
		ic.Welcomed = true

		// RPL_YOURHOST
		msg = fmt.Sprintf(
			":%s 002 %s :Your host is %s, running version %s\r\n",
			ic.Conn.LocalAddr(), ic.Nick, ic.Conn.LocalAddr(), VERSION)

		log.Printf(msg)
		_, err = ic.Conn.Write([]byte(msg))
		if err != nil {
			log.Fatal(err)
		}

		// RPL_CREATED
		msg = fmt.Sprintf(
			":%s 003 %s :This server was created %s\r\n",
			ic.Conn.LocalAddr(), ic.Nick, timeCreated)

		log.Printf(msg)
		_, err = ic.Conn.Write([]byte(msg))
		if err != nil {
			log.Fatal(err)
		}

		// RPL_MYINFO
		msg = fmt.Sprintf(
			":%s 004 %s %s %s %s %s\r\n",
			ic.Conn.LocalAddr(), ic.Nick, ic.Conn.LocalAddr(), VERSION, "ao", "mtov")

		log.Printf(msg)
		_, err = ic.Conn.Write([]byte(msg))
		if err != nil {
			log.Fatal(err)
		}

		writeLUsers(ic)
		writeMotd(ic)
	}
}

func validateWelcomeAndParameters(command, params string, expectedNumParams int, ic *IRCConn) bool {
	if command != "NICK" && command != "USER" && !ic.Welcomed {
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
