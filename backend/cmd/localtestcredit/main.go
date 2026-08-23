package main

import (
	"backend/config"
	"backend/data/models/user"
	"backend/services"
	"flag"
	"fmt"
	"log"
)

// localtestcredit is intentionally limited to the named local demo account.
// It uses the audited account service rather than changing balance directly.
func main() {
	amount := flag.Float64("amount", 10000000, "test credit amount in yuan")
	flag.Parse()
	if *amount <= 0 || *amount > 100000000 {
		log.Fatal("amount must be between 0 and 100,000,000")
	}
	config.LoadConfig()
	db, err := config.ConnectDB()
	if err != nil {
		log.Fatal(err)
	}
	var account user.User
	if err := db.Where("username = ?", "demo").First(&account).Error; err != nil {
		log.Fatalf("demo test account not found: %v", err)
	}
	updated, err := services.NewUserAdminService(db).AdjustBalance(
		account.UserID, *amount, "本地投注测试充值", "local-test-credit",
	)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("demo balance credited; current balance: %.2f\n", updated.Balance)
}
