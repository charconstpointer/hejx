package main

import (
	"log"
	"net"

	"github.com/go-irc/irc"
)

func main() {
	conn, err := net.Dial("tcp", "irc.twitch.tv:80")
	if err != nil {
		log.Fatalln(err)
	}

	config := irc.ClientConfig{
		Nick: "lolwhatwhy",
		Pass: "oauth:t8droh2a9ea27mmnoqftzvuofpslm8",
		User: "lolwhatwhy",
		Name: "lolwhatwhy2",
		Handler: irc.HandlerFunc(func(c *irc.Client, m *irc.Message) {
			log.Println("msg inc", m.Params)
			if m.Command == "001" {
				c.Write("JOIN #lolwhatwhy")
			} else if m.Command == "PRIVMSG" && c.FromChannel(m) {
				c.WriteMessage(&irc.Message{
					Command: "PRIVMSG",
					Params: []string{
						m.Params[0],
						m.Trailing(),
					},
				})
			}
		}),
	}

	// Create the client
	client := irc.NewClient(conn, config)
	err = client.Run()
	if err != nil {
		log.Fatalln(err)
	}
}
