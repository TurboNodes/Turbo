package supabase

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq" // Postgres driver
)

const dbOpTimeout = 8 * time.Second

var SupabaseDB *sqlx.DB

type UserData struct {
	AuthUserId string    `db:"authUserId"`
	CreatedAt  time.Time `db:"createdAt"`
	UpdatedAt  time.Time `db:"updatedAt"`
}

func InitDatabase(connString string) error {
	db, err := sqlx.Connect("postgres", connString)
	if err != nil {
		return err
	}
	SupabaseDB = db
	return nil
}

// CreateNodeConnectRequest inserts a pairing UUID for a node IP (TTL = 2 hours).
func CreateNodeConnectRequest(db *sqlx.DB, nodeIP string) (string, time.Time, error) {
	id := uuid.New().String()
	ctx, cancel := context.WithTimeout(context.Background(), dbOpTimeout)
	defer cancel()

	var expiresAt time.Time
	err := db.QueryRowContext(
		ctx,
		`INSERT INTO node_connect_requests (uuid, node_ip, expires_at)
		 VALUES ($1, $2, now() + interval '2 hours')
		 RETURNING expires_at`,
		id, nodeIP,
	).Scan(&expiresAt)
	if err != nil {
		return "", time.Time{}, err
	}
	return id, expiresAt, nil
}

// GetNodeUserID returns the owner of nodeIp, or "" if unpaired.
func GetNodeUserID(db *sqlx.DB, nodeIP string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), dbOpTimeout)
	defer cancel()

	var userID sql.NullString
	err := db.QueryRowContext(ctx, `SELECT "userId" FROM nodes WHERE "nodeIp" = $1`, nodeIP).Scan(&userID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	if !userID.Valid {
		return "", nil
	}
	return userID.String, nil
}

// SetNodeActive updates ops fields on nodes by nodeIp. Never touches userId.
func SetNodeActive(db *sqlx.DB, nodeIP string, active bool) error {
	ctx, cancel := context.WithTimeout(context.Background(), dbOpTimeout)
	defer cancel()

	_, err := db.ExecContext(
		ctx,
		`UPDATE nodes SET "isActive" = $1, "updatedAt" = $2 WHERE "nodeIp" = $3`,
		active, time.Now(), nodeIP,
	)
	return err
}
