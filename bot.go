// Package bot provides a simple to use IRC, Slack and Telegram bot
package bot

import (
	"errors"
	"log"
	"math/rand"
	"time"

	"github.com/robfig/cron"
)

const (
	// CmdPrefix is the prefix used to identify a command.
	// !hello would be identified as a command
	CmdPrefix = "!"

	// MsgBuffer is the max number of messages which can be buffered
	// while waiting to flush them to the chat service.
	MsgBuffer = 100
)

// Bot handles the bot instance
type Bot struct {
	handlers     *Handlers
	cron         *cron.Cron
	disabledCmds []string
	msgsToSend   chan responseMessage
	done         chan struct{}

	// Protocol and Server are used by MesssageStreams to
	/// determine if this is the correct bot to send a message on
	// see:
	// https://github.com/go-chat-bot/bot/issues/37#issuecomment-277661159
	// https://github.com/go-chat-bot/bot/issues/97#issuecomment-442827599
	Protocol string
	Server   string
}

// Config see above
type Config struct {
	Protocol string
	Server   string
}

type responseMessage struct {
	target, message string
	sender          *User
}

// ResponseHandler must be implemented by the protocol to handle the bot responses
type ResponseHandler func(target, message string, sender *User)

// ErrorHandler will be called when an error happens
type ErrorHandler func(msg string, err error)

// Handlers that must be registered to receive callbacks from the bot
type Handlers struct {
	Response ResponseHandler
	Errored  ErrorHandler
}

func logErrorHandler(msg string, err error) {
	log.Printf("%s: %s", msg, err.Error())
}

var singleton *Bot

// GetInstance hand back the Bot singleton
func GetInstance() (*Bot, error) {
	if singleton != nil {
		return singleton, nil
	}
	return nil, errors.New("bot has not been initialized")
}

// New configures a new bot instance
func New(h *Handlers, bc *Config) *Bot {
	if h.Errored == nil {
		h.Errored = logErrorHandler
	}

	b := &Bot{
		handlers:   h,
		cron:       cron.New(),
		msgsToSend: make(chan responseMessage, MsgBuffer),
		done:       make(chan struct{}),
		Protocol:   bc.Protocol,
		Server:     bc.Server,
	}

	// Launch the background goroutine that isolates the possibly non-threadsafe
	// message sending logic of the underlying transport layer.
	go b.processMessages()

	b.startMessageStreams()

	b.startPeriodicCommands()

	// store the Bot in a singleton to hand it back to anyone else who comes asking
	singleton = b
	return b
}

func (b *Bot) startMessageStreams() {
	for _, v := range messageStreamConfigs {

		go func(b *Bot, config *MessageStreamConfig) {
			msMap.Lock()
			ms := &MessageStream{
				Data: make(chan MessageStreamMessage),
				Done: make(chan bool),
			}
			var err = config.MsgFunc(ms)
			if err != nil {
				b.errored("MessageStream "+config.StreamName+" failed ", err)
			}
			msKey := messageStreamKey{
				Protocol:   b.Protocol,
				Server:     b.Server,
				StreamName: config.StreamName,
			}
			// thread safe write
			msMap.messageStreams[msKey] = ms
			msMap.Unlock()
			b.handleMessageStream(config.StreamName, ms)
		}(b, v)
	}
}

func (b *Bot) startPeriodicCommands() {
	for _, config := range periodicCommands {
		func(b *Bot, config PeriodicConfig) {
			b.cron.AddFunc(config.CronSpec, func() {
				switch config.Version {
				case v1:
					for _, channel := range config.Channels {
						message, err := config.CmdFunc(channel)
						if err != nil {
							b.errored("Periodic command failed ", err)
						} else if message != "" {
							cd := &ChannelData{Channel: channel, Server: b.Server, Protocol: b.Protocol}
							b.SendMessage(channel, cd, message, nil)
						}
					}
				case v2:
					results, err := config.CmdFuncV2()
					if err != nil {
						b.errored("Periodic command failed ", err)
						return
					}
					for _, result := range results {
						cd := &ChannelData{Channel: result.Channel, Server: b.Server, Protocol: b.Protocol}
						b.SendMessage(result.Channel, cd, result.Message, nil)
					}
				}
			})
		}(b, config)
	}
	if len(b.cron.Entries()) > 0 {
		b.cron.Start()
	}
}

// MessageReceived must be called by the protocol upon receiving a message
func (b *Bot) MessageReceived(channel *ChannelData, message *Message, sender *User) {
	command, err := parse(message.Text, channel, sender)
	if err != nil {
		b.SendMessage(channel.Channel, channel, err.Error(), sender)
		return
	}

	if (command != nil && command.Command != "") || message.Text != "" {
		// there there's something here to be filtered
		b.executeMessageReceiveFilters(message, command)
	}

	if command == nil {
		b.executePassiveCommands(&PassiveCmd{
			Raw:         message.Text,
			MessageData: message,
			Channel:     channel.Channel,
			ChannelData: channel,
			User:        sender,
		})
		return
	}

	if b.isDisabled(command.Command) {
		return
	}

	switch command.Command {
	case helpCommand:
		b.help(command)
	default:
		b.handleCmd(command)
	}
}

// SendMessage queues a message for a target recipient, optionally from a particular sender.
func (b *Bot) SendMessage(target string, cd *ChannelData, message string, sender *User) {
	// func (b *Bot) SendMessage(target string, message string, sender *User) {
	message = b.executeFilterCommands(&FilterCmd{
		Target:      target,
		ChannelData: cd,
		Message:     message,
		User:        sender})
	if message == "" {
		return
	}

	select {
	case b.msgsToSend <- responseMessage{target, message, sender}:
	default:
		b.errored("Failed to queue message to send.", errors.New("Too busy"))
	}
}

func (b *Bot) sendResponse(target, message string, sender *User) {
	b.handlers.Response(target, message, sender)
}

func (b *Bot) errored(msg string, err error) {
	if b.handlers.Errored != nil {
		b.handlers.Errored(msg, err)
	}
}

func (b *Bot) processMessages() {
	for {
		select {
		case msg := <-b.msgsToSend:
			b.sendResponse(msg.target, msg.message, msg.sender)
		case <-b.done:
			return
		}
	}
}

// Close will shut down the message sending capabilities of this bot. Call
// this when you are done using the bot.
func (b *Bot) Close() {
	close(b.done)
}

func init() {
	rand.Seed(time.Now().UnixNano())
}
