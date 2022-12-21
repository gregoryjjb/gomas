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
var Host = GetEnvOr("HOST", "")

// How many times per second to update the state of the lights
var FramesPerSecond float64 = 120
