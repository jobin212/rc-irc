# rc-irc
A custom IRC server built for recursers.

How to connect 
```shell
telnet localhost 8080
```

Run test locally:
```shell
 go build && mv rc-irc chirc && rm ~/chirc/build/chirc && cp ./chirc ~/chirc/build/ && cd ~/chirc/build && python3 -m pytest  ../tests/ -k test_connect_simple1 & cd ~/rc-irc

 go build && mv rc-irc chirc && rm ~/chirc/build/chirc && cp ./chirc ~/chirc/build/ && cd ~/chirc/build && python3 -m pytest  ../tests/ --chirc-category CONNECTION_REGISTRATION & cd ~/rc-irc

 go build && mv rc-irc chirc && rm ~/chirc/build/chirc && cp ./chirc ~/chirc/build/ && cd ~/chirc/build && python3 -m pytest  ../tests/ --chirc-category CHANNEL_PRIVMSG_NOTICE & cd ~/rc-irc

 go build && mv rc-irc chirc && rm ~/chirc/build/chirc && cp ./chirc ~/chirc/build/ && cd ~/chirc/build && make assignment-2 & cd ~/rc-irc
 ```