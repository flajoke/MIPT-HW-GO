package main

import (
	"context"
	"homework-8/ledger"
	"log"
	"time"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, closeFn, err := ledger.Open(ctx)
	if err != nil {
		log.Fatal(err)
	}
	if err := closeFn(); err != nil {
		log.Fatal(err)
	}
	log.Println("Ledger: PostgreSQL, Redis и схема БД готовы")
}
