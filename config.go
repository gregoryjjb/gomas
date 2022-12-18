package main

import "os"

func GetEnvOr(key string, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		value = fallback
	}
	return value
}

var Port = GetEnvOr("PORT", "1225")
var Host = GetEnvOr("HOST", "127.0.0.1")
