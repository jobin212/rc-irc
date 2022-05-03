package main

import (
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
)

func handleLUsers(ic *IRCConn, im IRCMessage) {
	//params := strings.Join(im.Params, " ")
	//if !validateParameters("LUSERS", params, 0, ic) {
	//	return
	//}

	writeLUsers(ic)
}

//func remove(s []int, i int) []int {
//    s[i] = s[len(s)-1]
//    return s[:len(s)-1]
//}

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

func handleWhoIs(ic *IRCConn, im IRCMessage) {
	params := strings.Join(im.Params, " ")
	targetNick := strings.Trim(params, " ")

	if targetNick == "" {
		return
	}

	targetIc, ok := lookupNickConn(targetNick)

	if !ok {
		rpl := replyMap["ERR_NOSUCHNICK"]
		msg, _ := formatReply(ic, rpl, []string{targetNick})
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

func handleDefault(ic *IRCConn, im IRCMessage) {
	command := im.Command
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

func handleNotice(ic *IRCConn, im IRCMessage) {
	params := strings.Join(im.Params, " ")
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
		":%s!%s@%s NOTICE %s :%s\r\n",
		ic.Nick, ic.User, ic.Conn.RemoteAddr(), targetNick, userMessage)
	_, err := recipientIc.Conn.Write([]byte(msg))
	if err != nil {
		log.Fatal(err)
	}

}

func handleMotd(ic *IRCConn, im IRCMessage) {
	writeMotd(ic)
}

func writeMotd(ic *IRCConn) {
	dat, err := os.ReadFile("./motd.txt")
	if err != nil {
		rpl := replyMap["ERR_NOMOTD"]
		msg, _ := formatReply(ic, rpl, []string{})
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

func handlePing(ic *IRCConn, im IRCMessage) {
	// TODO validate welcome?
	// TODO update ping to update connection lifetime?
	msg := fmt.Sprintf("PONG %s\r\n", ic.Conn.LocalAddr().String())
	_, err := ic.Conn.Write([]byte(msg))
	if err != nil {
		log.Fatal(err)
	}
}

func handlePong(ic *IRCConn, im IRCMessage) {
	return
}

func handlePrivMsg(ic *IRCConn, im IRCMessage) {
	params := strings.Join(im.Params, " ")
	//if !validateParameters("PRIVMSG", params, 2, ic) {
	//	return
	//}

	splitParams := strings.SplitN(params, " ", 2)
	target, userMessage := strings.Trim(splitParams[0], " "), splitParams[1]

	if strings.HasPrefix(target, "#") {
		// USER TO CHANNEL PM

		// get connection from targetNick
		channel, ok := lookupChannelByName(target)

		if !ok {
			rpl := replyMap["ERR_NOSUCHNICK"]
			msg, _ := formatReply(ic, rpl, []string{target})
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
			msg, _ := formatReply(ic, replyMap["ERR_CANNOTSENDTOCHAN"], []string{target})
			//msg := fmt.Sprintf(":%s 404 %s %s :Cannot send to channel\r\n", ic.Conn.LocalAddr(), ic.Nick, target)
			_, err := ic.Conn.Write([]byte(msg))
			if err != nil {
				log.Println("error sending nosuchnick reply")
			}
			return
		}

		msg := fmt.Sprintf(
			":%s!%s@%s PRIVMSG %s :%s\r\n",
			ic.Nick, ic.User, ic.Conn.RemoteAddr(), target, userMessage)
		if len(msg) > 512 {
			msg = msg[:510] + "\r\n"
		}

		sendMessageToChannel(ic, msg, channel, false)
	} else {
		// USER TO USER PM

		// get connection from targetNick
		recipientIc, ok := lookupNickConn(target)

		if !ok {
			rpl := replyMap["ERR_NOSUCHNICK"]
			msg, _ := formatReply(ic, rpl, []string{target})
			_, err := ic.Conn.Write([]byte(msg))
			if err != nil {
				log.Println("error sending nosuchnick reply")
			}
			return
		}

		msg := fmt.Sprintf(
			":%s!%s@%s PRIVMSG %s :%s\r\n",
			ic.Nick, ic.User, ic.Conn.RemoteAddr(), target, userMessage)
		if len(msg) > 512 {
			msg = msg[:510] + "\r\n"
		}
		_, err := recipientIc.Conn.Write([]byte(msg))
		if err != nil {
			log.Fatal(err)
		}

		if recipientIc.AwayMessage != "" {
			awayAutoReply := fmt.Sprintf(":%s!%s@%s 301 %s %s :%s\r\n",
				recipientIc.Nick, recipientIc.User, recipientIc.Conn.RemoteAddr(), ic.Nick, recipientIc.Nick, recipientIc.AwayMessage)
			_, err := ic.Conn.Write([]byte(awayAutoReply))
			if err != nil {
				log.Fatal(err)
			}
		}
	}
}

func handleQuit(ic *IRCConn, im IRCMessage) {
	params := strings.Join(im.Params, " ")
	//if !validateParameters("QUIT", params, 0, ic) {
	//	return
	//}

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

func handleNick(ic *IRCConn, im IRCMessage) {
	params := strings.Join(im.Params, " ")
	//if !validateParameters("NICK", params, 1, ic) {
	//	return
	//}

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

func handleUser(ic *IRCConn, im IRCMessage) {
	params := strings.Join(im.Params, " ")
	//if !validateParameters("USER", params, 4, ic) {
	//	return
	//}

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

func handleTopic(ic *IRCConn, im IRCMessage) {
	params := strings.Join(im.Params, " ")
	//if !validateParameters("TOPIC", params, 1, ic) {
	//	return
	//}

	splitParams := strings.SplitN(params, " ", 2)

	chanName := splitParams[0]
	newTopic := ""
	if len(splitParams) >= 2 {
		newTopic = removePrefix(splitParams[1])
	}

	ircCh, ok := lookupChannelByName(chanName)
	if !ok {
		// ERR Channel doesn't exist

		msg := fmt.Sprintf(":%s 442 %s %s :You're not on that channel\r\n", ic.Conn.LocalAddr(), ic.Nick, chanName)
		_, err := ic.Conn.Write([]byte(msg))
		if err != nil {
			log.Println("error sending ERR_NOTONCHANNEL reply")
		}
		return
	}

	memberOfChannel := false
	for _, v := range getChannelMembers(ircCh) {
		if v == ic {
			memberOfChannel = true
		}
	}

	if !memberOfChannel {
		msg := fmt.Sprintf(":%s 442 %s %s :You're not on that channel\r\n", ic.Conn.LocalAddr(), ic.Nick, chanName)
		_, err := ic.Conn.Write([]byte(msg))
		if err != nil {
			log.Println("error sending ERR_NOTONCHANNEL reply")
		}
		return
	}

	ircCh.Mtx.Lock()

	var msg string
	if newTopic != "" {
		// update channel topic
		ircCh.Topic = newTopic
		msg = fmt.Sprintf(":%s!%s@%s TOPIC %s :%s\r\n",
			ic.Nick, ic.User, ic.Conn.RemoteAddr(), chanName, ircCh.Topic)
	} else {
		// read channel topic
		if ircCh.Topic == "" {
			// RPL_NOTOPIC
			msg = fmt.Sprintf(":%s 331 %s %s :No topic is set\r\n", ic.Conn.LocalAddr(), ic.Nick, chanName)
		} else {
			// RPL_TOPIC
			msg = fmt.Sprintf(":%s 332 %s %s :%s\r\n", ic.Conn.LocalAddr(), ic.Nick, chanName, ircCh.Topic)
		}
	}
	ircCh.Mtx.Unlock()

	if newTopic != "" {
		sendMessageToChannel(ic, msg, ircCh, true)
	} else {
		_, err := ic.Conn.Write([]byte(msg))
		if err != nil {
			log.Println("error sending TOPIC reply")
		}
	}
}

func handleAway(ic *IRCConn, im IRCMessage) {
	params := strings.Join(im.Params, " ")
	awayMessage := removePrefix(strings.Trim(params, " "))
	var msg string
	if awayMessage == "" {
		// clear away message
		ic.AwayMessage = ""
		msg = fmt.Sprintf(":%s 305 %s :You are no longer marked as being away\r\n", ic.Conn.LocalAddr(), ic.Nick)
	} else {
		ic.AwayMessage = awayMessage
		msg = fmt.Sprintf(":%s 306 %s :You have been marked as being away\r\n", ic.Conn.LocalAddr(), ic.Nick)
	}
	_, err := ic.Conn.Write([]byte(msg))
	if err != nil {
		log.Println("error sending AWAY reply")
	}
}

func lookupChannelByName(name string) (*IRCChan, bool) {
	nameToChanMtx.Lock()
	ircCh, ok := nameToChan[name]
	nameToChanMtx.Unlock()
	return ircCh, ok
}

func sendMessageToChannel(senderIC *IRCConn, msg string, ircCh *IRCChan, sendToSelf bool) {
	members := getChannelMembers(ircCh)
	for _, v := range members {
		v := v
		if sendToSelf || v != senderIC {
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
	sendMessageToChannel(ic, joinMsg, ircCh, false)
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
	rplCode := 331
	if ircCh.Topic != "" {
		topic = ircCh.Topic
		rplCode = 332
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

func handlePart(ic *IRCConn, im IRCMessage) {
	params := strings.Join(im.Params, " ")
	//if !validateParameters("PART", params, 1, ic) {
	//	return
	//}
	splitParams := strings.SplitN(params, " ", 2) // maybe split on colon?
	chanName := splitParams[0]
	ircCh, ok := lookupChannelByName(chanName)
	if !ok {
		// ERR Channel doesn't exist
		msg := fmt.Sprintf(":%s 403 %s %s :No such channel\r\n", ic.Conn.LocalAddr(), ic.Nick, chanName)
		_, err := ic.Conn.Write([]byte(msg))
		if err != nil {
			log.Println("error sending nosuchnick reply")
		}
		return
	}

	memberOfChannel := false
	for _, v := range getChannelMembers(ircCh) {
		if v == ic {
			memberOfChannel = true
		}
	}

	if !memberOfChannel {
		msg, _ := formatReply(ic, replyMap["ERR_NOTONCHANNEL"], []string{chanName})
		_, err := ic.Conn.Write([]byte(msg))
		if err != nil {
			log.Println("error sending ERR_NOTONCHANNEL reply")
		}
		return
	}

	msg := ""
	if len(splitParams) == 2 {
		msg = fmt.Sprintf(":%s!%s@%s PART %s :%s\r\n", ic.Nick, ic.User, ic.Conn.RemoteAddr(), chanName, removePrefix(splitParams[1]))
	} else {
		msg = fmt.Sprintf(":%s!%s@%s PART %s\r\n", ic.Nick, ic.User, ic.Conn.RemoteAddr(), chanName)
	}

	sendMessageToChannel(ic, msg, ircCh, true)

	numChannelMembers := 0
	// Remove user from channel
	ircCh.Mtx.Lock()
	memberIndex := 0
	for i, v := range ircCh.Members {
		if v == ic {
			memberIndex = i
			break
		}
	}
	ircCh.Members[memberIndex] = ircCh.Members[len(ircCh.Members)-1]
	ircCh.Members = ircCh.Members[:len(ircCh.Members)-1]

	if ircCh.OpNicks[ic.Nick] {
		delete(ircCh.OpNicks, ic.Nick)
	}

	// Delete channel if nobody in it
	numChannelMembers = len(ircCh.Members)
	ircCh.Mtx.Unlock()
	channelIndex := 0
	if numChannelMembers == 0 {
		nameToChanMtx.Lock()
		chansMtx.Lock()
		delete(nameToChan, chanName)

		for i, v := range ircChans {
			if v == ircCh {
				channelIndex = i
				break
			}
		}
		ircChans[channelIndex] = ircChans[len(ircChans)-1]
		ircChans = ircChans[:len(ircChans)-1]
		chansMtx.Unlock()
		nameToChanMtx.Unlock()
	}
}

func handleJoin(ic *IRCConn, im IRCMessage) {
	params := strings.Join(im.Params, " ")
	//if !validateParameters("JOIN", params, 1, ic) {
	//	return
	//}
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

func handleList(ic *IRCConn, im IRCMessage) {
	var sb strings.Builder
	chansMtx.Lock()
	for _, ircChan := range ircChans {
		numMembers := len(getChannelMembers(ircChan))
		sb.WriteString(fmt.Sprintf(
			":%s 322 %s %s %d :%s\r\n",
			ic.Conn.LocalAddr(), ic.Nick, ircChan.Name, numMembers, ircChan.Topic))
	}
	sb.WriteString(fmt.Sprintf(
		":%s 323 %s :End of LIST\r\n",
		ic.Conn.LocalAddr(), ic.Nick))
	chansMtx.Unlock()
	_, err := ic.Conn.Write([]byte(sb.String()))
	if err != nil {
		log.Fatal(err)
	}
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
