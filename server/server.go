package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"

	"github.com/poodbooq/bitburst_server/config"
	"github.com/poodbooq/bitburst_server/logger"
	"github.com/poodbooq/bitburst_server/postgres"
	"github.com/poodbooq/bitburst_server/service"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cfg, err := config.Load()
	if err != nil {
		return
	}
	log, err := logger.Get(cfg.Logger)
	if err != nil {
		return
	}
	defer func() {
		if err := log.Close(); err != nil {
			fmt.Println(err)
		}
	}()

	database, err := postgres.Load(ctx, cfg.Postgres, log)
	if err != nil {
		return
	}
	defer func() {
		if err := database.Close(); err != nil {
			fmt.Println(err)
		}
	}()

	go service.
		Load(database, log, cfg.Service).
		Run(ctx)

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, os.Kill)
	<-sig
	fmt.Println("closing")
}
