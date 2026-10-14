package main

import (
	"log"
	"os"
	"runtime"
	"strconv"
)

const mebibyte = 1024 * 1024

func main() {
	targetMiB := 128
	if value := os.Getenv("BATCH_MEMORY_MB"); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed < 1 {
			log.Fatalf("BATCH_MEMORY_MB must be a positive integer, got %q", value)
		}
		targetMiB = parsed
	}

	// Retain and touch each memory page so the operating system accounts for it.
	memory := make([]byte, targetMiB*mebibyte)
	for page := 0; page < len(memory); page += 4096 {
		memory[page] = byte(page / 4096)
	}
	log.Printf("batch started; holding about %d MiB and consuming CPU", targetMiB)

	// Keep changing a value and touching the allocated buffer in an endless loop.
	var counter uint64
	for {
		counter = counter*1664525 + 1013904223
		index := int(counter % uint64(len(memory)))
		memory[index] = byte(counter)
		runtime.KeepAlive(memory)
	}
}
