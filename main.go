package main

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"log"
	"math/big"
	"net"
	"net/http"
	"os"
	"server/data/database"
	"server/data/database/supabase"
	"server/discord"
	"server/proxy"
	"server/website"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func generateTLSCert() tls.Certificate {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		log.Fatal(err)
	}

	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization: []string{"Turbo Proxy Dev"},
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(time.Hour * 24 * 180), // Valid for 180 days
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1")},
		DNSNames:              []string{"localhost"},
	}

	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		log.Fatal(err)
	}

	cert := tls.Certificate{
		Certificate: [][]byte{certDER},
		PrivateKey:  key,
	}

	return cert
}

func main() {
	database.InitRedis()
	if err := supabase.InitDatabase(os.Getenv("DATABASE_URL")); err != nil {
		log.Fatalf("Supabase initialization failed: %v", err)
	}
	if err := database.InitClickHouse(); err != nil {
		log.Printf("Warning: ClickHouse initialization failed: %v", err)
	}

	http.Handle("/metrics", promhttp.Handler())
	http.HandleFunc("/stats", website.StatsHandler)
	go func() {
		if err := http.ListenAndServe(":8080", nil); err != nil {
			log.Fatal("Failed to start Prometheus metrics endpoint:", err)
		}
	}()

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{generateTLSCert()},
		NextProtos:   []string{"turbo-proxy"}, // Application protocol
	}
	log.Println("Starting QUIC server on :8443")
	err := proxy.StartQuicServer(":8443", tlsConfig)
	if err != nil {
		log.Fatal("Failed to start QUIC server:", err)
	}

	log.Println("Starting SOCKS5 receiver on :1080")
	listener, err := net.Listen("tcp", ":1080")
	if err != nil {
		log.Fatal("Failed to start SOCKS5 receiver:", err)
	}
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				log.Printf("Couldn't accept SOCKS5 connection: %v", err)
				continue
			}
			go proxy.HandleSocksConn(conn)
		}
	}()

	if os.Getenv("DISCORD_TOKEN") != "" {
		discord.StartBot(os.Getenv("DISCORD_TOKEN"))
		log.Println("Started Discord bot")
	}

	select {}
}
