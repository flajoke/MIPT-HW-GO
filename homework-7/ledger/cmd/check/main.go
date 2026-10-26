package main

import (
	"context"
	"homework-7/ledger"
	"log"
	"time"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := ledger.Init(ctx); err != nil {
		log.Fatal(err)
	}
	if err := ledger.Close(); err != nil {
		log.Fatal(err)
	}
	log.Println("Ledger: PostgreSQL, Redis и схема БД готовы")
}
