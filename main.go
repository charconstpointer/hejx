package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/go-irc/irc"
	"github.com/kelseyhightower/envconfig"
	"golang.org/x/sync/errgroup"
)

type Config struct {
	Name string `envconfig:"IRC_NAME" default:"lolwhatwhy"`
	Pass string `envconfig:"IRC_PASS" required:"true"`
	Nick string `envconfig:"IRC_NICK" default:"lolwhatwhy"`
	User string `envconfig:"IRC_USER" default:"lolwhatwhy"`
	Host string `envconfig:"IRC_HOST" default:"irc.twitch.tv:80"`
	Chan string `envconfig:"IRC_CHANNEL" default:"lolwhatwhy"`
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

func (h *Handler) Run(ctx context.Context, channels ...string) error {
	conn, err := net.Dial("tcp", h.cfg.Host)
	if err != nil {
		return fmt.Errorf("failed to dial: %v", err)
	}
	defer conn.Close()

	for _, ch := range channels {
		h.logs[ch] = make(chan *Message)
	}

	eg, ctx := errgroup.WithContext(ctx)
	eg.SetLimit(len(channels))
	for _, ch := range channels {
		ch := ch
		if ok := eg.TryGo(func() error {
			return h.listenChan(ctx, ch)
		}); !ok {
			return fmt.Errorf("failed to start channel listener: %s", ch)
		}
	}

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
					User:    m.Prefix.Name,
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
			return fn(msg)
		}
	}
}

func main() {
	var cfg Config
	if err := envconfig.Process("", &cfg); err != nil {
		log.Fatalf("failed to process config: %v", err)
	}

	ctx := context.Background()
	ctx, cancel := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer cancel()

	channels := []string{"#lolwhatwhy", "#asmongold"}

	eg, ctx := errgroup.WithContext(ctx)
	eg.SetLimit(2)

	handler := NewHandler(&cfg)
	handler.Handle("#lolwhatwhy", func(m *Message) error {
		log.Printf("[%s@%s]: %s\n", m.User, m.Channel, m.Text)
		return nil
	})

	if ok := eg.TryGo(func() error {
		if err := handler.Run(ctx, channels...); err != nil {
			log.Fatalf("failed to run handler: %v", err)
		}
		return nil
	}); !ok {
		log.Fatalf("failed to start handler")
	}

	<-ctx.Done()
}
