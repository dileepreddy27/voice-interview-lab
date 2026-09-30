package main

import (
	"log"
	"net/http"
	"os"
	"time"

	"github.com/dileepreddy27/voice-interview-lab/internal/gateway"
)

func main() {
	addr := os.Getenv("GATEWAY_ADDR")
	if addr == "" {
		addr = "127.0.0.1:8080"
	}
	coach := os.Getenv("COACH_URL")
	if coach == "" {
		coach = "http://127.0.0.1:8091"
	}
	s := gateway.New(coach, os.Getenv("DEEPGRAM_API_KEY"), "web")
	log.Printf("Voice Interview Lab listening on %s", addr)
	log.Fatal((&http.Server{Addr: addr, Handler: s.Handler(), ReadHeaderTimeout: 5 * time.Second}).ListenAndServe())
}
