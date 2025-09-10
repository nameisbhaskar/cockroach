// Copyright 2025 The Cockroach Authors.
//
// Use of this software is governed by the CockroachDB Software License
// included in the /LICENSE file.

package testselector

import (
	"context"
	"crypto/rsa"
	"crypto/x509"
	gosql "database/sql"
	"encoding/pem"
	"fmt"
	"os"
	"strconv"

	sf "github.com/snowflakedb/gosnowflake"
)

const (
	account   = "lt53838.us-central1.gcp"
	database  = "DATAMART_PROD"
	schema    = "TEAMCITY"
	warehouse = "COMPUTE_WH"

	// sfUsernameEnv and sfPrivateKey are the environment variables that are used for Snowflake access
	sfUsernameEnv = "SNOWFLAKE_USER"
	sfPrivateKey  = "SNOWFLAKE_PVT_KEY"
)

// SqlConnectorFunc is the function to get a sql connector
var SqlConnectorFunc = gosql.Open

// supported suites
var suites = map[string]string{
	"nightly": "roachtest nightly",
}

// getConnect makes connection to snowflake and returns the connection.
func getConnect(_ context.Context) (*gosql.DB, error) {
	dsn, err := getDSN()
	if err != nil {
		return nil, err
	}
	db, err := SqlConnectorFunc("snowflake", dsn)
	if err != nil {
		return nil, err
	}
	return db, nil
}

// getDSN returns the dataSource name for snowflake driver
func getDSN() (string, error) {
	username, privateKeyStr, err := getSFCreds()
	if err != nil {
		return "", err
	}
	privateKey, err := loadPrivateKey(privateKeyStr)
	if err != nil {
		return "", err
	}
	return sf.DSN(&sf.Config{
		Account:       account,
		Database:      database,
		Schema:        schema,
		Warehouse:     warehouse,
		Authenticator: sf.AuthTypeJwt,
		User:          username,
		PrivateKey:    privateKey,
	})
}

// getSFCreds gets the snowflake credentials from the secrets manager
func getSFCreds() (string, string, error) {
	username := os.Getenv(sfUsernameEnv)
	privateKey := os.Getenv(sfPrivateKey)
	if username == "" {
		return "", "", fmt.Errorf("environment variable %s is not set", sfUsernameEnv)
	}
	if privateKey == "" {
		return "", "", fmt.Errorf("environment variable %s is not set", sfPrivateKey)
	}
	return username, privateKey, nil
}

// loadPrivateKey loads an RSA private key by parsing the PEM-encoded key bytes.
func loadPrivateKey(privateKeyString string) (privateKey *rsa.PrivateKey, err error) {
	block, _ := pem.Decode([]byte(privateKeyString))
	if block == nil {
		return nil, fmt.Errorf("failed to decode PEM block containing the key")
	}

	var parsedKey interface{}
	if parsedKey, err = x509.ParsePKCS8PrivateKey(block.Bytes); err != nil {
		parsedKey, err = x509.ParsePKCS1PrivateKey(block.Bytes)
		if err != nil {
			return nil, err
		}
	}

	privateKey, ok := parsedKey.(*rsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("not an RSA private key")
	}

	return privateKey, nil
}

// getDuration extracts the duration from the snowflake query duration field
func getDuration(durationStr string) int64 {
	duration, _ := strconv.ParseInt(durationStr, 10, 64)
	return duration
}
