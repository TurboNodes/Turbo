package database

import (
	"github.com/redis/go-redis/v9"
)

func PublishFeatures(features map[string]string) {
	_ = rdb.XAdd(ctx,
		&redis.XAddArgs{
			Stream: "connections",
			Values: features,
		},
	).Err()

	return
}
