package api

import (
	"database/sql"
	"fmt"
	"log"

	_ "github.com/lib/pq"
	"github.com/redis/go-redis/v9"
)

func ConnectPostgres() *sql.DB {
	connStr := "host=localhost port=5432 user=traffic password=traffic dbname=trafficdb sslmode=disable"
	db, err := sql.Open("postgres", connStr)
	if err != nil {
		log.Fatal("Failed to open Postgres connection:", err)
	}

	err = db.Ping()
	if err != nil {
		log.Fatal("Failed to ping Postgres:", err)
	}

	fmt.Println("Connected to Postgres!")
	return db
}

func ConnectRedis() *redis.Client {
	client := redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
	})

	fmt.Println("Connected to Redis!")
	return client
}
