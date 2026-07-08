package socks

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
)

const (
	SocksVersion = 5

	NoAuth       = 0x00
	UserPassAuth = 0x02

	ConnectCommand = 0x01

	IPv4Address = 0x01
	FQDN        = 0x03
	IPv6Address = 0x04

	SuccessReply   = 0x00
	GeneralFailure = 0x01
)

func HandleSocksHandshake(conn net.Conn) (string, int, map[string]string, error) {
	header := make([]byte, 2)
	if _, err := io.ReadFull(conn, header); err != nil {
		return "", 0, nil, err
	}

	if header[0] != SocksVersion {
		return "", 0, nil, errors.New(fmt.Sprintf("unsupported SOCKS version %d", header[0]))
	}

	authMethods := make([]byte, header[1])
	if _, err := io.ReadFull(conn, authMethods); err != nil {
		return "", 0, nil, err
	}

	authSupported := false
	for _, method := range authMethods {
		if method == UserPassAuth {
			authSupported = true
			break
		}
	}

	if !authSupported {
		return "", 0, nil, errors.New(fmt.Sprintf("no valid supported auth methods, received %v", authMethods))
	}

	response := []byte{SocksVersion, UserPassAuth}
	if _, err := conn.Write(response); err != nil {
		return "", 0, nil, err
	}

	authenticated, params, err := Authenticate(conn)
	if err != nil || !authenticated {
		return "", 0, nil, fmt.Errorf("authentication failed: %e", err)
	}

	request := make([]byte, 4)
	if _, err := io.ReadFull(conn, request); err != nil {
		return "", 0, nil, err
	}

	if request[0] != SocksVersion {
		return "", 0, nil, errors.New("invalid SOCKS version in request")
	}

	if request[1] != ConnectCommand {
		return "", 0, nil, errors.New("only CONNECT command is supported")
	}

	var targetAddr string
	var targetPort int

	switch request[3] {
	case IPv4Address:
		addr := make([]byte, 4)
		if _, err := io.ReadFull(conn, addr); err != nil {
			return "", 0, nil, err
		}
		targetAddr = net.IPv4(addr[0], addr[1], addr[2], addr[3]).String()

	case FQDN:
		lenByte := make([]byte, 1)
		if _, err := io.ReadFull(conn, lenByte); err != nil {
			return "", 0, nil, err
		}
		fqdnLen := int(lenByte[0])
		fqdn := make([]byte, fqdnLen)
		if _, err := io.ReadFull(conn, fqdn); err != nil {
			return "", 0, nil, err
		}
		targetAddr = string(fqdn)

	case IPv6Address:
		addr := make([]byte, 16)
		if _, err := io.ReadFull(conn, addr); err != nil {
			return "", 0, nil, err
		}
		targetAddr = net.IP(addr).String()

	default:
		return "", 0, nil, errors.New("unsupported address type")
	}

	portBytes := make([]byte, 2)
	if _, err := io.ReadFull(conn, portBytes); err != nil {
		return "", 0, nil, err
	}
	targetPort = int(binary.BigEndian.Uint16(portBytes))

	return targetAddr, targetPort, params, nil
}
