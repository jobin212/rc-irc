package main

import (
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
)

func handleLUsers(ic *IRCConn, params string) {
	if !validateParameters("LUSERS", params, 0, ic) {
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
	if command == "" || !ic.Welcomed {
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
	// TODO handle channels
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

func handlePong(ic *IRCConn, params string) {
	return
}

func handlePrivMsg(ic *IRCConn, params string) {
	if !validateParameters("PRIVMSG", params, 2, ic) {
		return
	}

	splitParams := strings.SplitN(params, " ", 2)
	target, userMessage := strings.Trim(splitParams[0], " "), splitParams[1]

	if strings.HasPrefix(target, "#") {
		// get connection from targetNick
		channel, ok := lookupChannelByName(target)

		if !ok {
			msg := fmt.Sprintf(":%s 401 %s %s :No such nick/channel\r\n", ic.Conn.LocalAddr(), ic.Nick, target)
			_, err := ic.Conn.Write([]byte(msg))
			if err != nil {
				log.Println("error sending nosuchnick reply")
			}
			return
		}

		memberOfChannel := false
		for _, v := range getChannelMembers(channel) {
			if v == ic {
				memberOfChannel = true
			}
		}

		if !memberOfChannel {
			msg := fmt.Sprintf(":%s 404 %s %s :Cannot send to channel\r\n", ic.Conn.LocalAddr(), ic.Nick, target)
			_, err := ic.Conn.Write([]byte(msg))
			if err != nil {
				log.Println("error sending nosuchnick reply")
			}
			return
		}

		msg := fmt.Sprintf(
			":%s!%s@%s PRIVMSG %s %s\r\n",
			ic.Nick, ic.User, ic.Conn.RemoteAddr(), target, userMessage)
		if len(msg) > 512 {
			msg = msg[:510] + "\r\n"
		}

		sendMessageToChannel(ic, msg, channel)
	} else {
		// get connection from targetNick
		recipientIc, ok := lookupNickConn(target)

		if !ok {
			msg := fmt.Sprintf(":%s 401 %s %s :No such nick/channel\r\n", ic.Conn.LocalAddr(), ic.Nick, target)
			_, err := ic.Conn.Write([]byte(msg))
			if err != nil {
				log.Println("error sending nosuchnick reply")
			}
			return
		}

		msg := fmt.Sprintf(
			":%s!%s@%s PRIVMSG %s %s\r\n",
			ic.Nick, ic.User, ic.Conn.RemoteAddr(), target, userMessage)
		if len(msg) > 512 {
			msg = msg[:510] + "\r\n"
		}
		_, err := recipientIc.Conn.Write([]byte(msg))
		if err != nil {
			log.Fatal(err)
		}
	}
}

func handleQuit(ic *IRCConn, params string) {
	if !validateParameters("QUIT", params, 0, ic) {
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
	if !validateParameters("NICK", params, 1, ic) {
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
	if !validateParameters("USER", params, 4, ic) {
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

func sendMessageToChannel(senderIC *IRCConn, msg string, ircCh *IRCChan) {
	members := getChannelMembers(ircCh)
	for _, v := range members {
		v := v
		if v != senderIC {
			go func() {
				_, err := v.Conn.Write([]byte(msg))

				if err != nil {
					log.Fatal(err)
				}
			}()
		}
	}
}

func addUserToChannel(ic *IRCConn, ircCh *IRCChan) {
	ircCh.Mtx.Lock()
	ircCh.Members = append(ircCh.Members, ic)
	lm := len(ircCh.Members)
	var members = make([]*IRCConn, lm, lm)
	copy(members, ircCh.Members)
	ircCh.Mtx.Unlock()
	joinMsg := fmt.Sprintf(":%s!%s@%s JOIN %s\r\n", ic.Nick, ic.User, ic.Conn.RemoteAddr(), ircCh.Name)
	sendMessageToChannel(ic, joinMsg, ircCh)
	_, err := ic.Conn.Write([]byte(joinMsg))
	if err != nil {
		log.Fatal(err)
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

func sendTopicReply(ic *IRCConn, ircCh *IRCChan) {
	// Send channel topic to ic
	// if channel topic is not sent, send RPL_NOTOPIC instead
	topic := "No topic is set"
	rplCode := 332
	if ircCh.Topic != "" {
		topic = ircCh.Topic
		rplCode = 331
	} else {
		return
	}
	topicReply := fmt.Sprintf(":%s %03d %s %s :%s\r\n", ic.Conn.LocalAddr(), rplCode, ic.Nick, ircCh.Name, topic)
	_, err := ic.Conn.Write([]byte(topicReply))
	if err != nil {
		log.Fatal(err)
	}
}

func getChannelMembers(ircCh *IRCChan) []*IRCConn {
	ircCh.Mtx.Lock()
	lm := len(ircCh.Members)
	members := make([]*IRCConn, lm, lm)
	copy(members, ircCh.Members)
	ircCh.Mtx.Unlock()
	return members
}

func sendNamReply(ic *IRCConn, ircCh *IRCChan) {
	var sb strings.Builder
	channelStatusIndicator := "=" // Using public indicator as default
	sb.WriteString(fmt.Sprintf(":%s %03d %s %s %s :", ic.Conn.LocalAddr(), 353, ic.Nick,
		channelStatusIndicator, ircCh.Name))
	members := getChannelMembers(ircCh)
	for i, v := range members {
		n := v.Nick
		if i != 0 {
			sb.WriteString(" ")
		}
		present, ok := ircCh.OpNicks[n]
		if present && ok {
			// append "@" to indicate channel member is op
			sb.WriteString("@")
		}
		// append channel member nick
		sb.WriteString(n)
	}
	sb.WriteString("\r\n")
	// Send RPL_NAMREPLY
	_, err := ic.Conn.Write([]byte(sb.String()))
	if err != nil {
		log.Fatal(err)
	}

	endOfNames := fmt.Sprintf(":%s %03d %s %s :End of NAMES list\r\n", ic.Conn.LocalAddr(), 366, ic.Nick, ircCh.Name)
	_, err = ic.Conn.Write([]byte(endOfNames))
	if err != nil {
		log.Fatal(err)
	}
}

func handleJoin(ic *IRCConn, params string) {
	if !validateParameters("JOIN", params, 1, ic) {
		return
	}
	chanName := params

	ircCh, ok := lookupChannelByName(chanName)
	if !ok {
		// Create new channel
		ircCh = newChannel(ic, chanName)
	}

	members := getChannelMembers(ircCh)
	for _, v := range members {
		if v == ic {
			return
		}
	}
	// Join channel
	addUserToChannel(ic, ircCh)
	// RPL_TOPIC
	sendTopicReply(ic, ircCh)
	// RPL_NAMREPLY & RPL_ENDOFNAMES
	sendNamReply(ic, ircCh)
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
