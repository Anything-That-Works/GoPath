package handler

import (
	"net"
	"net/http"

	"github.com/Anything-That-Works/GoPath/internal/model"
	"github.com/Anything-That-Works/GoPath/internal/ratelimit"
	"golang.org/x/time/rate"
)

// 30 requests per second, burst of 60
var httpLimiter = ratelimit.NewLimiter(rate.Limit(30), 60)

func MiddlewareRateLimit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip, _, _ := net.SplitHostPort(r.RemoteAddr)
		if !httpLimiter.Allow(ip) {
			respondWithJSON(w, 429, model.APIResponse{
				Success: false,
				Message: "Too many requests, please slow down",
			})
			return
		}
		next.ServeHTTP(w, r)
	})
}
