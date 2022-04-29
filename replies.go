package main

import "fmt"

// 001 RPL_WELCOME
func rpl_welcome(host, nick, user, remoteHost string) string {
	return fmt.Sprintf(
		":%s 001 %s :Welcome to the Internet Relay Network %s!%s@%s\r\n",
		host, nick, nick, user, remoteHost)
}

// 002 RPL_YOURHOST
func rpl_yourhost(host, nick string) string {
	return fmt.Sprintf(
		":%s 002 %s :Your host is %s, running version %s\r\n",
		host, nick, host, VERSION)
}

// 003 RPL_CREATED
func rpl_created(host, nick string) string {
	return fmt.Sprintf(
		":%s 003 %s :This server was created %s\r\n",
		host, nick, timeCreated)
}

// 004 RPL_MYINFO
func rpl_myinfo(host, nick, userMode, channelMode string) string {
	return fmt.Sprintf(
		":%s 004 %s %s %s %s %s\r\n",
		host, nick, host, VERSION, userMode, channelMode)
}
