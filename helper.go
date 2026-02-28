package main

import (
	"net"
	"net/http"
	"strings"

	"github.com/sqlc-dev/pqtype"
)

func getIPAddress(r *http.Request) pqtype.Inet {
	ip := r.Header.Get("X-Forwarded-For")

	if ip != "" {
		// take first IP if multiple
		ip = strings.Split(ip, ",")[0]
		ip = strings.TrimSpace(ip)
	}

	if ip == "" {
		ip = r.Header.Get("X-Real-IP")
	}

	if ip == "" {
		host, _, err := net.SplitHostPort(r.RemoteAddr)
		if err == nil {
			ip = host
		}
	}

	parsed := net.ParseIP(ip)
	if parsed == nil {
		return pqtype.Inet{Valid: false}
	}

	// correct mask for IPv4 vs IPv6
	maskSize := 32
	if parsed.To4() == nil {
		maskSize = 128
	}

	return pqtype.Inet{
		IPNet: net.IPNet{
			IP:   parsed,
			Mask: net.CIDRMask(maskSize, maskSize),
		},
		Valid: true,
	}
}

func handlerError(w http.ResponseWriter, r *http.Request) {
	respondWithError(w, 400, "Something went wrong!")
}

func handlerReadiness(w http.ResponseWriter, r *http.Request) {
	respondWithJSON(w, 200, struct{}{})
}
