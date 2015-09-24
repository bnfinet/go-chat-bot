package irc

import (
	"crypto/tls"
	"log"
	"strings"

	"github.com/go-chat-bot/bot"
	ircevent "github.com/thoj/go-ircevent"
)

// Config must contain the necessary data to connect to an IRC server
type Config struct {
	Server        string   // IRC server:port. Ex: ircevent.freenode.org:7000
	Channels      []string // Channels to connect. Ex: []string{"#go-bot", "#channel mypassword"}
	User          string   // The IRC username the bot will use
	Nick          string   // The nick the bot will use
	Password      string   // Server password
	UseTLS        bool     // Should connect using TLS?
	TLSServerName string   // Must supply if UseTLS is true
	Debug         bool     // This will log all IRC communication to standad output
}

func messageHandler(target, message, sender string) {
	channel := target
	if conn.GetNick() == target {
		channel = sender
	}
	conn.Privmsg(channel, message)
}

var (
	conn   *ircevent.Connection
	gobot  *bot.Bot
	config *Config
)

func onPRIVMSG(e *ircevent.Event) {
	gobot.MessageReceived(e.Arguments[0], e.Message(), e.Nick)
}

func getServerName(server string) string {
	separatorIndex := strings.LastIndex(server, ":")
	if separatorIndex != -1 {
		return server[:separatorIndex]
	}
	return server
}

func onWelcome(e *ircevent.Event) {
	for _, channel := range config.Channels {
		conn.Join(channel)
	}
}

// Run reads the Config, connect to the specified IRC server and starts the bot.
// The bot will automatically join all the channels specified in the configuration
func Run(c *Config) {
	config = c

	ircConn := ircevent.IRC(c.User, c.Nick)
	ircConn.Password = c.Password
	ircConn.UseTLS = c.UseTLS
	ircConn.TLSConfig = &tls.Config{
		ServerName: getServerName(c.Server),
	}
	ircConn.VerboseCallbackHandler = c.Debug
	err := ircConn.Connect(c.Server)
	if err != nil {
		log.Fatal(err)
	}

	ircConn.AddCallback("001", onWelcome)
	ircConn.AddCallback("PRIVMSG", onPRIVMSG)

	gobot = bot.NewBot(messageHandler, c.Channels)

	ircConn.Loop()
}