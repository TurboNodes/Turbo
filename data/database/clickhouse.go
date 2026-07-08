package database

import (
	"context"
	"fmt"
	"os"

	"github.com/ClickHouse/clickhouse-go/v2"
)

var ClickHouseConn clickhouse.Conn

func InitClickHouse() error {
	chAddr := os.Getenv("CLICKHOUSE_ADDR")
	if chAddr == "" {
		chAddr = "localhost:9000"
	}

	chDatabase := os.Getenv("CLICKHOUSE_DATABASE")
	if chDatabase == "" {
		chDatabase = "default"
	}

	chUser := os.Getenv("CLICKHOUSE_USER")
	if chUser == "" {
		chUser = "default"
	}

	chPassword := os.Getenv("CLICKHOUSE_PASSWORD")
	if chPassword == "" {
		chPassword = ""
	}

	conn, err := clickhouse.Open(&clickhouse.Options{
		Addr: []string{chAddr},
		Auth: clickhouse.Auth{
			Username: chUser,
			Password: chPassword,
			Database: chDatabase,
		},
	})

	if err != nil {
		return fmt.Errorf("failed to connect to ClickHouse: %w", err)
	}

	// Test the connection
	if err := conn.Ping(context.Background()); err != nil {
		return fmt.Errorf("failed to ping ClickHouse: %w", err)
	}

	ClickHouseConn = conn
	return nil
}
