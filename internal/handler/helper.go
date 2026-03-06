package handler

import (
	"net"
	"net/http"
	"strings"

	"github.com/Anything-That-Works/GoPath/internal/model"
	"github.com/sqlc-dev/pqtype"
)

func (handler *Handler) getIPAddress(r *http.Request) pqtype.Inet {
	remoteIP, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return pqtype.Inet{Valid: false}
	}

	// only trust forwarded headers if request is from trusted proxy
	if handler.ApiConfig.TrustedProxy != "" && remoteIP == handler.ApiConfig.TrustedProxy {
		if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
			// X-Forwarded-For can be a comma separated list — take the first
			if idx := len(fwd); idx > 0 {
				for i := 0; i < len(fwd); i++ {
					if fwd[i] == ',' {
						fwd = fwd[:i]
						break
					}
				}
			}
			ip := net.ParseIP(strings.TrimSpace(fwd))
			if ip != nil {
				return buildInet(ip)
			}
		}

		if realIP := r.Header.Get("X-Real-IP"); realIP != "" {
			ip := net.ParseIP(strings.TrimSpace(realIP))
			if ip != nil {
				return buildInet(ip)
			}
		}
	}

	// fall back to direct remote address
	ip := net.ParseIP(remoteIP)
	if ip == nil {
		return pqtype.Inet{Valid: false}
	}
	return buildInet(ip)
}

func buildInet(ip net.IP) pqtype.Inet {
	bits := 32
	if ip.To4() == nil {
		bits = 128 // IPv6
	}
	return pqtype.Inet{
		IPNet: net.IPNet{
			IP:   ip,
			Mask: net.CIDRMask(bits, bits),
		},
		Valid: true,
	}
}

func HandlerError(w http.ResponseWriter, r *http.Request) {
	respondWithError(w, 400, "Something went wrong!")
}

func HandlerReadiness(w http.ResponseWriter, r *http.Request) {
	respondWithJSON(w, 200, model.APIResponse{
		Success: true,
		Message: "Ready",
	})
}
