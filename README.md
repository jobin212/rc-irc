# rc-irc
A custom IRC server built for recursers.

How to connect 
```shell
telnet localhost 8080
```

Run tests
- [Setup chirc](http://chi.cs.uchicago.edu/chirc/installing.html#software-requirements)
- [Setup chirc testing](http://chi.cs.uchicago.edu/chirc/testing.html#using-the-automated-tests)
```shell
💬 go build && mv rc-irc ~/rc-irc/chirc/build/chirc
💬 cd ./chirc/build/
💬 make assignment-3
💬 python3 -m pytest ../tests/ --chirc-category PRIVMSG_NOTICE
 ```