// Default echo backend: returns request metadata as JSON. Used as
// default_app in the gateway's CDS/LDS - the catch-all cluster any route
// without a more specific backend falls through to.
package main

import (
	"encoding/json"
	"io"
	"log"
	"net"
	"net/http"
	"os"
)

type response struct {
	OK      string      `json:"ok"`
	Request requestData `json:"request"`
}

type requestData struct {
	RemoteIP string              `json:"remote_ip"`
	Method   string              `json:"method"`
	Path     string              `json:"path"`
	Headers  map[string][]string `json:"headers"`
	Query    *string             `json:"query"`
	Body     string              `json:"body"`
}

func echo(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)

	var query *string
	if q := r.URL.RawQuery; q != "" {
		query = &q
	}

	remoteIP := r.RemoteAddr
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		remoteIP = host
	}

	data := requestData{
		RemoteIP: remoteIP,
		Method:   r.Method,
		Path:     r.URL.Path,
		Headers:  r.Header,
		Query:    query,
		Body:     string(body),
	}

	log.Printf("[app] %+v", data)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response{OK: "true", Request: data})
}

func main() {
	port := "5050"
	if len(os.Args) > 1 {
		port = os.Args[1]
	} else if p := os.Getenv("PORT"); p != "" {
		port = p
	}

	http.HandleFunc("/", echo)
	addr := "0.0.0.0:" + port
	log.Printf("HTTP server listening on http://%s", addr)
	log.Fatal(http.ListenAndServe(addr, nil))
}
