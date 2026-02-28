package main

import (
	"net"
	"net/http"

	"github.com/sqlc-dev/pqtype"
)

func getIPAddress(r *http.Request) pqtype.Inet {
	ip := r.Header.Get("X-Forwarded-For")
	if ip == "" {
		ip = r.Header.Get("X-Real-IP")
	}
	if ip == "" {
		ip, _, _ = net.SplitHostPort(r.RemoteAddr)
	}

	parsed := net.ParseIP(ip)
	if parsed == nil {
		return pqtype.Inet{Valid: false}
	}
	return pqtype.Inet{
		IPNet: net.IPNet{
			IP:   parsed,
			Mask: net.CIDRMask(32, 32),
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
