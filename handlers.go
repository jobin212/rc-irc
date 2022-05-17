package main

import (
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
)

func userCanSetTopic(ic *IRCConn, ircCh *IRCChan) bool {
	ircCh.Mtx.Lock()
	tr := ircCh.isTopicRestricted
	ircCh.Mtx.Unlock()
	if tr {
		return userIsChannelOp(ic, ircCh)
	}

	return true
}

func setMemberStatusMode(nick, mode string, ircCh *IRCChan) (int, error) {
	if ircCh == nil {
		return -1, fmt.Errorf("setMemberStatusMode - ircCh is nil")
	}
	ircCh.Mtx.Lock()
	defer ircCh.Mtx.Unlock()
	modeChange := mode[0]
	modeType := mode[1]
	v := false

	switch modeChange {
	case '+':
		v = true
	case '-':
		v = false
	default:
		return 2, fmt.Errorf("setMemberStatusMode - unknown modechange - %s", mode)
	}

	switch modeType {
	case 'v':
		ircCh.CanTalk[nick] = v
	case 'o':
		ircCh.OpNicks[nick] = v
	default:
		return 2, fmt.Errorf("setMemberStatusMode - unknown modetype - %s", mode)
	}

	return 0, nil
}

func userIsChannelOp(ic *IRCConn, ircCh *IRCChan) bool {
	ircCh.Mtx.Lock()
	defer ircCh.Mtx.Unlock()
	isOp, ok := ircCh.OpNicks[ic.Nick]
	return isOp && ok
}

func setChannelMode(ircCh *IRCChan, mode string) error {
	if len(mode) != 2 {
		return fmt.Errorf("setChannelMode - invalid mode")
	}
	modeChange := mode[0]
	modeType := mode[1]
	modeValue := false

	switch modeChange {
	case '+':
		modeValue = true
	case '-':
		modeValue = false
	default:
		return fmt.Errorf("setChannelMode - invalid mode")
	}

	ircCh.Mtx.Lock()

	switch modeType {
	case 'm':
		ircCh.isModerated = modeValue
	case 't':
		ircCh.isTopicRestricted = modeValue
	default:
		return fmt.Errorf("setChannelMode - invalid mode")
	}
	defer ircCh.Mtx.Unlock()

	return nil
}

func getChannelMode(ircCh *IRCChan) (string, error) {
	ircCh.Mtx.Lock()
	channelMode := ""
	if ircCh.isModerated {
		channelMode += "m"
	}
	if ircCh.isTopicRestricted {
		channelMode += "t"
	}
	defer ircCh.Mtx.Unlock()
	return channelMode, nil
}

func handleMode(ic *IRCConn, im IRCMessage) error {
	// target can be nick or channel
	fmt.Println("in handleMode")
	target := im.Params[0]
	if strings.HasPrefix(target, "#") {
		fmt.Println("target has prefix #")
		// dealing with channel
		chanName := target
		ircCh, ok := lookupChannelByName(chanName)
		if !ok {
			msg, _ := formatReply(ic, replyMap["ERR_NOSUCHCHANNEL"], []string{chanName})
			return sendMessage(ic, msg)
			// channel doesn't exist
		}
		switch len(im.Params) {
		case 1:
			// Requesting channel mode
			channelMode, _ := getChannelMode(ircCh)
			msg := fmt.Sprintf(":%s!%s@%s 324 %s %s +%s\r\n", ic.Nick, ic.User, ic.Conn.LocalAddr(), ic.Nick, chanName, channelMode)
			return sendMessage(ic, msg)
		case 2:
			// Modifying channel mode
			fmt.Println("params has length 2")
			mode := im.Params[1]
			if userIsChannelOp(ic, ircCh) {
				err := setChannelMode(ircCh, mode)
				msg := ""
				if err != nil {
					modeChar := string(mode[1])
					msg, _ = formatReply(ic, replyMap["ERR_UNKNOWNMODE"], []string{modeChar, chanName})
				} else {
					msg = fmt.Sprintf(":%s!%s@%s MODE %s %s\r\n", ic.Nick, ic.User, ic.Conn.LocalAddr(), chanName, mode)
					sendMessageToChannel(ic, msg, ircCh, false)
				}
				return sendMessage(ic, msg)

			} else {
				fmt.Printf("handleMode - Handling else case in len im params 2\n")
				msg, _ := formatReply(ic, replyMap["ERR_CHANOPRIVSNEEDED"], []string{chanName})
				fmt.Printf("formatReply returned\n")
				return sendMessage(ic, msg)
			}
		case 3:
			fmt.Println("params has length 3")
			// Modifying channelMemberStatus
			mode := im.Params[1]
			nick := im.Params[2]
			// TODO check that sender can set modes
			if !userIsChannelOp(ic, ircCh) {
				msg, _ := formatReply(ic, replyMap["ERR_CHANOPRIVSNEEDED"], []string{chanName})
				return sendMessage(ic, msg)
			}
			// check if user is in channel
			ircCh.Mtx.Lock()
			nickIsChannelMember := false
			for _, v := range ircCh.Members {
				if v.Nick == nick {
					nickIsChannelMember = true
					break
				}
			}
			ircCh.Mtx.Unlock()
			if !nickIsChannelMember {
				msg, _ := formatReply(ic, replyMap["ERR_USERNOTINCHANNEL"], []string{nick, chanName})
				return sendMessage(ic, msg)
			}

			fmt.Println("setting member status mode")
			// set Mode
			rv, err := setMemberStatusMode(nick, mode, ircCh)
			if err != nil {
				switch rv {
				case 2:
					modeChar := string(mode[1])
					msg, _ := formatReply(ic, replyMap["ERR_UNKNOWNMODE"], []string{modeChar, chanName})
					return sendMessage(ic, msg)
				}
			}

			fmt.Println("sending message to channel")
			rpl := fmt.Sprintf(":%s!%s@%s %s %s %s %s\r\n", ic.Nick, ic.User, ic.Conn.LocalAddr(), im.Command, im.Params[0], im.Params[1], im.Params[2])
			sendMessageToChannel(ic, rpl, ircCh, true)

		default: // invalid num params
		}
	} else {
		// dealing with user nick
		nick := target
		if nick != ic.Nick {
			msg, _ := formatReply(ic, replyMap["ERR_USERSDONTMATCH"], []string{})
			return sendMessage(ic, msg)
			// Send some error
		}
		mode := im.Params[1]
		if len(mode) != 2 {
			rpl, _ := replyMap["ERR_UMODEUNKNOWNFLAG"]
			msg, _ := formatReply(ic, rpl, []string{})
			return sendMessage(ic, msg)
		}
		modeChange := mode[0]
		modeType := mode[1]
		modeValue := false
		switch modeChange {
		case '+':
			modeValue = true
		case '-':
			modeValue = false
		default:
			rpl, _ := replyMap["ERR_UMODEUNKNOWNFLAG"]
			msg, _ := formatReply(ic, rpl, []string{})
			return sendMessage(ic, msg)
		}
		switch modeType {
		case 'o':
			//return nil
			//fmt.Println("Handling operator case")
			//ic.isOperator = modeValue
			if modeValue {
				return nil
			} else {
				rpl := fmt.Sprintf(":%s %s %s :%s\r\n", ic.Nick, im.Command, im.Params[0], im.Params[1])
				return sendMessage(ic, rpl)
			}

		case 'a':
			return nil
		default:
			rpl, _ := replyMap["ERR_UMODEUNKNOWNFLAG"]
			msg, _ := formatReply(ic, rpl, []string{})
			return sendMessage(ic, msg)
		}
	}
	return nil
}

func handleLUsers(ic *IRCConn, im IRCMessage) error {
	writeLUsers(ic)
	return nil
}

func writeLUsers(ic *IRCConn) error {
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

	err := sendMessage(ic, sb.String())
	if err != nil {
		return fmt.Errorf("writeLusers - %w", err)
	}
	return nil
}

func handleWhoIs(ic *IRCConn, im IRCMessage) error {
	params := strings.Join(im.Params, " ")
	targetNick := strings.Trim(params, " ")

	if targetNick == "" {
		return nil
	}

	targetIc, ok := lookupNickConn(targetNick)

	if !ok {
		rpl := replyMap["ERR_NOSUCHNICK"]
		msg, _ := formatReply(ic, rpl, []string{targetNick})
		err := sendMessage(ic, msg)
		if err != nil {
			return fmt.Errorf("whoIs - sending NOSUCHNICK - %w", err)
		}
		return nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf(":%s 311 %s %s %s %s * :%s\r\n",
		ic.Conn.LocalAddr(), ic.Nick, targetIc.Nick, targetIc.User, targetIc.Conn.RemoteAddr().String(), targetIc.RealName))

	sb.WriteString(fmt.Sprintf(":%s 312 %s %s %s :%s\r\n",
		ic.Conn.LocalAddr(), ic.Nick, targetIc.Nick, targetIc.Conn.LocalAddr().String(), "<server info>"))

	sb.WriteString(fmt.Sprintf(":%s 318 %s %s :End of WHOIS list\r\n",
		ic.Conn.LocalAddr(), ic.Nick, targetIc.Nick))

	err := sendMessage(ic, sb.String())
	if err != nil {
		return fmt.Errorf("whoIs - %w", err)
	}

	return nil
}

func handleDefault(ic *IRCConn, im IRCMessage) error {
	command := im.Command
	if command == "" || !ic.Welcomed {
		return nil
	}

	rplName := "ERR_UNKNOWNCOMMAND"
	msg, _ := formatReply(ic, replyMap[rplName], []string{command})
	err := sendMessage(ic, msg)
	if err != nil {
		return fmt.Errorf("handleDefault - sending %s %w", rplName, err)
	}
	return nil
}

// Helper function to deal with the fact that nickToConn should be threadsafe
func lookupNickConn(nick string) (*IRCConn, bool) {
	nickToConnMtx.Lock()
	recipientIc, ok := nickToConn[nick]
	nickToConnMtx.Unlock()
	return recipientIc, ok
}

func handleNotice(ic *IRCConn, im IRCMessage) error {
	params := strings.Join(im.Params, " ")
	// TODO handle channels
	splitParams := strings.SplitN(params, " ", 2)
	if len(splitParams) < 2 {
		return nil
	}
	targetNick, userMessage := splitParams[0], splitParams[1]

	// get connection from targetNick
	recipientIc, ok := lookupNickConn(targetNick)

	if !ok {
		// TODO this should probably log something
		return nil
	}

	msg := fmt.Sprintf(
		":%s!%s@%s NOTICE %s :%s\r\n",
		ic.Nick, ic.User, ic.Conn.RemoteAddr(), targetNick, userMessage)
	err := sendMessage(recipientIc, msg)
	if err != nil {
		return fmt.Errorf("handleNotice - %w", err)
	}
	return nil
}

func handleMotd(ic *IRCConn, im IRCMessage) error {
	return writeMotd(ic)
}

func writeMotd(ic *IRCConn) error {
	dat, err := os.ReadFile("./motd.txt")
	if err != nil {
		rplName := "ERR_NOMOTD"
		rpl := replyMap[rplName]
		msg, _ := formatReply(ic, rpl, []string{})
		err := sendMessage(ic, msg)
		if err != nil {
			return fmt.Errorf("writeMotd - sending %s %w", rplName, err)
		}
		return nil
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

	err = sendMessage(ic, sb.String())
	if err != nil {
		return fmt.Errorf("writeMotd - sending MOTD %w", err)
	}
	return nil
}

func handlePing(ic *IRCConn, im IRCMessage) error {
	// TODO validate welcome?
	// TODO update ping to update connection lifetime?
	msg := fmt.Sprintf("PONG %s\r\n", ic.Conn.LocalAddr().String())
	err := sendMessage(ic, msg)
	if err != nil {
		return fmt.Errorf("handlePing - %w", err)
	}
	return nil
}

func handlePong(ic *IRCConn, im IRCMessage) error {
	return nil
}

func handlePrivMsg(ic *IRCConn, im IRCMessage) error {
	params := strings.Join(im.Params, " ")

	splitParams := strings.SplitN(params, " ", 2)
	target, userMessage := strings.Trim(splitParams[0], " "), splitParams[1]

	if strings.HasPrefix(target, "#") {
		// USER TO CHANNEL PM

		// get connection from targetNick
		channel, ok := lookupChannelByName(target)

		if !ok {
			rplName := "ERR_NOSUCHNICK"
			rpl := replyMap[rplName]
			msg, _ := formatReply(ic, rpl, []string{target})
			err := sendMessage(ic, msg)
			if err != nil {
				return fmt.Errorf("handlePrivMsg - writing %s - %w", rplName, err)
			}
			return nil
		}

		memberOfChannel := false
		for _, v := range getChannelMembers(channel) {
			if v == ic {
				memberOfChannel = true
			}
		}

		if !memberOfChannel {
			rplName := "ERR_CANNOTSENDTOCHAN"
			msg, _ := formatReply(ic, replyMap[rplName], []string{target})
			err := sendMessage(ic, msg)
			if err != nil {
				return fmt.Errorf("handlePrivMsg - writing %s - %w", rplName, err)
			}
			return nil
		}

		if channel.isModerated {
			fmt.Println("in channel isModerated block")
			channel.Mtx.Lock()
			senderCanTalk, ok := channel.CanTalk[ic.Nick]
			channel.Mtx.Unlock()

			if !(senderCanTalk && ok) {
				fmt.Println("!senderCanTalk && ok")
				rplName := "ERR_CANNOTSENDTOCHAN"
				msg, _ := formatReply(ic, replyMap[rplName], []string{target})
				err := sendMessage(ic, msg)
				if err != nil {
					return fmt.Errorf("handlePrivMsg - writing %s - %w", rplName, err)
				}
			}
		}

		msg := fmt.Sprintf(
			":%s!%s@%s PRIVMSG %s :%s\r\n",
			ic.Nick, ic.User, ic.Conn.RemoteAddr(), target, userMessage)
		if len(msg) > 512 {
			msg = msg[:510] + "\r\n"
		}

		fmt.Printf("sending message to channel: %s", msg)
		sendMessageToChannel(ic, msg, channel, false)
	} else {
		// USER TO USER PM

		// get connection from targetNick
		recipientIc, ok := lookupNickConn(target)

		if !ok {
			rplName := "ERR_NOSUCHNICK"
			rpl := replyMap[rplName]
			msg, _ := formatReply(ic, rpl, []string{target})
			err := sendMessage(ic, msg)
			if err != nil {
				return fmt.Errorf("handlePrivMsg - writing %s - %w", rplName, err)
			}
			return nil
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
			err := sendMessage(ic, awayAutoReply)
			if err != nil {
				return fmt.Errorf("handlePrivMsg - writing awayAutoReply - %w", err)
			}
		}
	}
	return nil
}

func handleQuit(ic *IRCConn, im IRCMessage) error {
	params := strings.Join(im.Params, " ")

	quitMessage := "Client Quit"
	if params != "" {
		quitMessage = removePrefix(params)
	}

	msg := fmt.Sprintf("ERROR :Closing Link: %s (%s)\r\n", ic.Conn.RemoteAddr(), quitMessage)
	err := sendMessage(ic, msg)
	if err != nil {
		return fmt.Errorf("handleQuit - sending closing link - %w", err)
	}

	cleanupIC(ic)

	//nickToConnMtx.Lock()
	//delete(nickToConn, ic.Nick)
	//nickToConnMtx.Unlock()

	//connsMtx.Lock()
	//for idx, conn := range ircConns {
	//	if conn == ic {
	//		ircConns = append(ircConns[:idx], ircConns[idx+1:]...)
	//		break
	//	}
	//}
	//connsMtx.Unlock()

	ic.isDeleted = true
	err = ic.Conn.Close()
	if err != nil {
		return fmt.Errorf("handleQuit - Closing connection - %w", err)
	}
	return nil
}

// CONTINUE FROM HERE
func handleNick(ic *IRCConn, im IRCMessage) error {
	params := strings.Join(im.Params, " ")

	prevNick := ic.Nick
	nick := strings.SplitN(params, " ", 2)[0]

	_, nickInUse := lookupNickConn(nick)
	if nick != ic.Nick && nickInUse { // TODO what happens if they try to change their own nick?
		msg, _ := formatReply(ic, replyMap["ERR_NICKNAMEINUSE"], []string{nick})
		_, err := ic.Conn.Write([]byte(msg))
		if err != nil {
			log.Fatal(err)
		}
		return nil
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
	return nil
}

func handleUser(ic *IRCConn, im IRCMessage) error {
	params := strings.Join(im.Params, " ")

	if ic.Welcomed && ic.User != "" {
		msg := fmt.Sprintf(
			":%s 463 :You may not reregister\r\n",
			ic.Conn.LocalAddr())
		_, err := ic.Conn.Write([]byte(msg))
		if err != nil {
			log.Fatal(err)
		}

		return nil
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
	return nil
}

func handleTopic(ic *IRCConn, im IRCMessage) error {
	params := strings.Join(im.Params, " ")

	splitParams := strings.SplitN(params, " ", 2)

	chanName := splitParams[0]
	newTopic := ""
	if len(splitParams) >= 2 {
		newTopic = removePrefix(splitParams[1])
	}

	ircCh, ok := lookupChannelByName(chanName)
	if !ok {
		// ERR Channel doesn't exist

		msg, _ := formatReply(ic, replyMap["ERR_NOTONCHANNEL"], []string{chanName}) // This is required by tests
		_, err := ic.Conn.Write([]byte(msg))
		if err != nil {
			log.Println("error sending ERR_NOTONCHANNEL reply")
		}
		return nil
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
		return nil
	}

	if !userCanSetTopic(ic, ircCh) {
		msg, _ := formatReply(ic, replyMap["ERR_CHANOPRIVSNEEDED"], []string{chanName})
		return sendMessage(ic, msg)
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
	return nil
}

func handleAway(ic *IRCConn, im IRCMessage) error {
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
	return nil
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

// TODO need to clean up cantalk and opnick status when user leaves channel
func newChannel(ic *IRCConn, chanName string) *IRCChan {
	newChan := IRCChan{
		Mtx:     sync.Mutex{},
		Name:    chanName,
		Topic:   "",
		OpNicks: make(map[string]bool),
		CanTalk: make(map[string]bool),
		Members: []*IRCConn{},
	}
	newChan.OpNicks[ic.Nick] = true
	newChan.CanTalk[ic.Nick] = true
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

func handlePart(ic *IRCConn, im IRCMessage) error {
	params := strings.Join(im.Params, " ")
	splitParams := strings.SplitN(params, " ", 2) // maybe split on colon?
	chanName := splitParams[0]
	ircCh, ok := lookupChannelByName(chanName)
	if !ok {
		// ERR Channel doesn't exist
		msg, _ := formatReply(ic, replyMap["ERR_NOSUCHCHANNEL"], []string{chanName})
		_, err := ic.Conn.Write([]byte(msg))
		if err != nil {
			log.Println("error sending nosuchnick reply")
		}
		return nil
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
		return nil
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
	return nil
}

func handleJoin(ic *IRCConn, im IRCMessage) error {
	params := strings.Join(im.Params, " ")
	chanName := params

	ircCh, ok := lookupChannelByName(chanName)
	if !ok {
		// Create new channel
		ircCh = newChannel(ic, chanName)
	}

	members := getChannelMembers(ircCh)
	for _, v := range members {
		if v == ic {
			return nil
		}
	}
	// Join channel
	addUserToChannel(ic, ircCh)
	// RPL_TOPIC
	sendTopicReply(ic, ircCh)
	// RPL_NAMREPLY & RPL_ENDOFNAMES
	sendNamReply(ic, ircCh)
	return nil
}

func handleList(ic *IRCConn, im IRCMessage) error {
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
	return nil
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
