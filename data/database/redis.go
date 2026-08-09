package database

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"

	"github.com/redis/go-redis/v9"
	"golang.org/x/crypto/bcrypt"
)

var (
	rdb *redis.Client
	ctx = context.Background()
)

func InitRedis() {
	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr == "" {
		redisAddr = "localhost:6379"
	}

	rdb = redis.NewClient(&redis.Options{
		Addr: redisAddr,
	})
}

// Deprecated: distant registration
func RegisterUser(password string, credits int) error {
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("error hashing password: %v", err)
	}

	hashKey := string(hashedPassword)
	rdb.HSet(ctx, "key:"+hashKey, "credits", credits)

	return nil
}

func GetCredits(password string) (int, error) {
	if os.Getenv("NO_REDIS") == "1" {
		log.Println("NO_REDIS is enabled, skipping Redis account credits check")
		return 999, nil
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return 0, fmt.Errorf("error hashing password: %v", err)
	}

	hashKey := string(hashedPassword)
	credits, err := rdb.HGet(ctx, "key:"+hashKey, "credits").Int()

	if err != nil {
		if errors.Is(err, redis.Nil) {
			return 0, fmt.Errorf("invalid credentials")
		} else {
			return 0, fmt.Errorf("error retrieving credits: %v", err)
		}
	}

	return credits, nil
}
