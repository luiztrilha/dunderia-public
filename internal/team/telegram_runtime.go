package team

import (
	"context"
	"errors"
	"log"
	"time"

	"github.com/nex-crm/wuphf/internal/config"
)

const telegramTransportRetryDelay = 10 * time.Second

func (l *Launcher) startTelegramTransportLoop() {
	if l == nil || l.broker == nil || l.telegramCancel != nil {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	l.telegramCancel = cancel

	go func() {
		for {
			token := config.ResolveTelegramBotToken()
			if token != "" && len(l.broker.SurfaceChannels("telegram")) > 0 {
				transport := NewTelegramTransport(l.broker, token)
				err := transport.Start(ctx)
				if errors.Is(err, context.Canceled) || ctx.Err() != nil {
					return
				}
				if err != nil {
					log.Printf("telegram transport stopped: %v", err)
				}
			}

			select {
			case <-ctx.Done():
				return
			case <-time.After(telegramTransportRetryDelay):
			}
		}
	}()
}
