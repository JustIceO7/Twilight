package db_client

import (
	"fmt"
	"os"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

var (
	DB *gorm.DB
)

func Init() {
	dsn := "postgres://postgres:postgres@postgres:5432/postgres"

	var err error
	for range 10 {
		DB, err = gorm.Open(postgres.Open(dsn), &gorm.Config{})
		if err == nil {
			sqlDB, _ := DB.DB()
			if pingErr := sqlDB.Ping(); pingErr == nil {
				break
			}
		}
		fmt.Println("Waiting for Postgres to be ready...")
		time.Sleep(time.Second)
	}
	if err != nil {
		fmt.Println("Unable to connect to database:", err)
		return
	}

	schema, err := os.ReadFile("db_client/schema.sql")
	if err != nil {
		fmt.Println("Unable to read schema.sql:", err)
		return
	}

	if err := DB.Exec(string(schema)).Error; err != nil {
		fmt.Println("Unable to execute schema.sql:", err)
	}
}
