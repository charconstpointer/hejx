package irc

import (
	"context"
	"fmt"
	"log"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/go-irc/irc"
	"golang.org/x/sync/errgroup"
)

type Config struct {
	Name  string   `envconfig:"IRC_NAME" default:"lolwhatwhy"`
	Pass  string   `envconfig:"IRC_PASS" required:"true"`
	Nick  string   `envconfig:"IRC_NICK" default:"lolwhatwhy"`
	User  string   `envconfig:"IRC_USER" default:"lolwhatwhy"`
	Host  string   `envconfig:"IRC_HOST" default:"irc.twitch.tv:80"`
	Chans []string `envconfig:"IRC_CHANNELS" default:"lolwhatwhy,asmongold"`
}

type Message struct {
	User    string
	Channel string
	Text    string
}

type HandlerFunc func(m *Message) error
type Handler struct {
	mu       sync.Mutex
	cfg      *Config
	c        *irc.Client
	logs     map[string]chan *Message
	handlers map[string]HandlerFunc
}

func NewHandler(cfg *Config) *Handler {
	return &Handler{
		cfg:      cfg,
		logs:     make(map[string]chan *Message),
		handlers: make(map[string]HandlerFunc),
	}
}

func (h *Handler) Run(ctx context.Context) error {
	var channels []string
	for _, ch := range h.cfg.Chans {
		if strings.HasPrefix(ch, "#") {
			channels = append(channels, ch)
		} else {
			channels = append(channels, fmt.Sprintf("#%s", ch))
		}
	}
	if len(channels) == 0 {
		return fmt.Errorf("no channels specified")
	}

	conn, err := net.Dial("tcp", h.cfg.Host)
	if err != nil {
		return fmt.Errorf("failed to dial: %v", err)
	}
	defer conn.Close()

	h.mu.Lock()
	for _, ch := range channels {
		h.logs[ch] = make(chan *Message)
	}
	h.mu.Unlock()

	eg, ctx := errgroup.WithContext(ctx)
	eg.SetLimit(len(channels))

	h.mu.Lock()
	for _, ch := range channels {
		ch := ch
		if ok := eg.TryGo(func() error {
			return h.listenChan(ctx, ch)
		}); !ok {
			return fmt.Errorf("failed to start channel listener: %s", ch)
		}
	}
	h.mu.Unlock()

	ircCfg := irc.ClientConfig{
		Nick: h.cfg.Nick,
		Pass: h.cfg.Pass,
		User: h.cfg.User,
		Name: h.cfg.Name,
		Handler: irc.HandlerFunc(func(c *irc.Client, m *irc.Message) {
			if m.Command == "001" {
				for _, ch := range channels {
					log.Println("trying to join channel:", ch)
					c.Write(fmt.Sprint("JOIN ", ch))
					log.Println("joined channel:", ch)
				}
			} else if m.Command == "PRIVMSG" && c.FromChannel(m) {
				channelName := m.Params[0]
				text := strings.Join(m.Params[1:], " ")
				m := &Message{
					User:    m.Name,
					Channel: channelName,
					Text:    text,
				}
				if ch, ok := h.logs[channelName]; ok {
					ch <- m
				} else {
					h.logs[channelName] = make(chan *Message)
					channel := h.logs[channelName]
					channel <- m
					log.Println("📝", channelName)
				}
			}
		}),
	}

	client := irc.NewClient(conn, ircCfg)
	if err = client.RunContext(ctx); err != nil {
		return fmt.Errorf("failed to run client: %v", err)
	}

	return nil
}

func (h *Handler) Handle(channel string, fn HandlerFunc) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, ok := h.handlers[channel]; ok {
		log.Printf("handler already exists for channel: %s\n", channel)
		return
	}
	h.handlers[channel] = fn
}

func (h *Handler) listenChan(ctx context.Context, channelName string) error {
	channel, ok := h.logs[channelName]
	for !ok {
		channel, ok = h.logs[channelName]
		time.Sleep(time.Second)
	}
	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("context canceled")
		case msg := <-channel:
			fn, ok := h.handlers["#lolwhatwhy"]
			if !ok {
				return fmt.Errorf("handler not found for channel: %s", channelName)
			}
			if err := fn(msg); err != nil {
				return fmt.Errorf("failed to handle message: %v", err)
			}
		}
	}
}
