package socks

import (
	"errors"
	"io"
	"net"
	"os"
	"server/data/database"
	"server/proxy/user"
)

func Authenticate(conn net.Conn) (bool, map[string]string, error) {
	// Read auth version (must be 0x01)
	header := make([]byte, 1)
	if _, err := io.ReadFull(conn, header); err != nil {
		return false, nil, err
	}

	if header[0] != 0x01 {
		return false, nil, errors.New("unsupported auth version")
	}

	userLenBuf := make([]byte, 1)
	if _, err := io.ReadFull(conn, userLenBuf); err != nil {
		return false, nil, err
	}
	userLen := userLenBuf[0]

	userBuf := make([]byte, userLen)
	if _, err := io.ReadFull(conn, userBuf); err != nil {
		return false, nil, err
	}
	username := string(userBuf)
	// username is actually the type of proxy , e.g. residential

	passLenBuf := make([]byte, 1)
	if _, err := io.ReadFull(conn, passLenBuf); err != nil {
		return false, nil, err
	}
	passLen := passLenBuf[0]

	passBuf := make([]byte, passLen)
	if _, err := io.ReadFull(conn, passBuf); err != nil {
		return false, nil, err
	}
	password := string(passBuf)

	credits, err := database.GetCredits(password)
	_ = credits
	// TODO: create local user struct to consume credits

	// Authentication response: version 0x01 + status
	if err != nil && os.Getenv("DEBUG_MODE") != "1" { // TODO: replace debug mode by default creds in redis
		conn.Write([]byte{0x01, GeneralFailure})
		return false, nil, err
	}

	conn.Write([]byte{0x01, SuccessReply})

	return true, user.ParseParams(username), nil
}
