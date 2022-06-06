package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/charconstpointer/hej/irc"
	"github.com/common-nighthawk/go-figure"
	"github.com/kelseyhightower/envconfig"
	"golang.org/x/sync/errgroup"
)

func main() {
	var cfg irc.Config
	if err := envconfig.Process("", &cfg); err != nil {
		log.Fatalf("failed to process config: %v", err)
	}

	myFigure := figure.NewColorFigure("I guess?", "", "red", true)
	myFigure.Print()

	ctx := context.Background()
	ctx, cancel := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer cancel()

	eg, ctx := errgroup.WithContext(ctx)
	eg.SetLimit(2)

	handler := irc.NewHandler(&cfg)
	handler.Handle("#lolwhatwhy", func(m *irc.Message) error {
		log.Printf("[%s@%s]: %s\n", m.User, m.Channel, m.Text)
		return nil
	})

	if ok := eg.TryGo(func() error {
		if err := handler.Run(ctx); err != nil {
			log.Fatalf("failed to run handler: %v", err)
		}
		return nil
	}); !ok {
		log.Fatalf("failed to start handler")
	}

	<-ctx.Done()
}
