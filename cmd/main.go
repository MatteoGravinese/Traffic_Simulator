package main

import (
	"fmt"

	"github.com/MatteoGravinese/Traffic_Simulator/internal/api"
)

func main() {
	fmt.Println("Traffic Simulator starting...")
	db := api.ConnectPostgres()
	defer db.Close()
	rdb := api.ConnectRedis()
	_ = rdb
	fmt.Println("All systems ready.")
}
