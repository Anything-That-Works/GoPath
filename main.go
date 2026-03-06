package main

import (
	"context"
	"database/sql"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/Anything-That-Works/GoPath/internal/cache"
	"github.com/Anything-That-Works/GoPath/internal/database"
	"github.com/Anything-That-Works/GoPath/internal/handler"
	"github.com/Anything-That-Works/GoPath/internal/model"
	"github.com/Anything-That-Works/GoPath/internal/storage"
	"github.com/Anything-That-Works/GoPath/internal/ws"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/cors"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
)

func requireEnv(key string) string {
	val := os.Getenv(key)
	if val == "" {
		log.Fatalf("%s value not found.", key)
	}
	return val
}

func main() {
	if err := godotenv.Load(".env"); err != nil {
		log.Println("No .env file found, using environment variables")
	}

	portString := requireEnv("PORT")
	dbURL := requireEnv("DB_URL")
	jwtSecretKey := requireEnv("JWT_SECRET_KEY")
	allowedOrigins := requireEnv("ALLOWED_ORIGINS")
	uploadsPath := requireEnv("UPLOADS_PATH")
	baseURL := requireEnv("BASE_URL")
	trustedProxy := os.Getenv("TRUSTED_PROXY")
	redisURL := requireEnv("REDIS_URL")

	redisCache, err := cache.NewRedisCache(redisURL)
	if err != nil {
		log.Fatal("Cannot connect to Redis: ", err)
	}
	defer func() {
		if err := redisCache.Close(); err != nil {
			log.Printf("Error closing Redis connection: %v", err)
		}
	}()

	con, err := sql.Open("postgres", dbURL)
	if err != nil {
		log.Fatal("Cannot connect to database: ", err)
	}
	defer func() {
		if err := con.Close(); err != nil {
			log.Printf("Error closing database connection: %v", err)
		}
	}()

	con.SetMaxOpenConns(50)
	con.SetMaxIdleConns(10)
	con.SetConnMaxLifetime(5 * time.Minute)
	con.SetConnMaxIdleTime(2 * time.Minute)

	if err := con.Ping(); err != nil {
		log.Fatal("Cannot ping database: ", err)
	}

	hub := ws.NewHub()
	go hub.Run()

	apiConfig := model.ApiConfig{
		DB:           database.New(con),
		JWTSecretKey: []byte(jwtSecretKey),
		Storage:      storage.NewLocalStorage(uploadsPath, baseURL),
		Hub:          hub,
		TrustedProxy: trustedProxy,
		Cache:        redisCache,
	}
	h := handler.New(&apiConfig)
	msgHandler := ws.NewMessageHandler(hub, h.ApiConfig.DB, h.ApiConfig.Storage)

	router := chi.NewRouter()

	router.Use(cors.Handler(cors.Options{
		AllowedOrigins:   strings.Split(allowedOrigins, ","),
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Authorization", "Content-Type"},
		ExposedHeaders:   []string{"Link"},
		AllowCredentials: false,
		MaxAge:           300,
	}))

	router.Use(handler.MiddlewareRateLimit)

	// security headers
	router.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Content-Type-Options", "nosniff")
			w.Header().Set("X-Frame-Options", "DENY")
			w.Header().Set("X-XSS-Protection", "1; mode=block")
			w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
			w.Header().Set("Content-Security-Policy",
				"default-src 'self'; "+
					"connect-src 'self' ws: wss: https:; "+
					"img-src 'self' data: https:; "+
					"media-src 'self' https:",
			)
			next.ServeHTTP(w, r)
		})
	})

	// 1MB body limit middleware
	limitMiddleware := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r.Body = http.MaxBytesReader(w, r.Body, 1*1024*1024)
			next.ServeHTTP(w, r)
		})
	}

	// write timeout middleware
	writeTimeoutMiddleware := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			rc := http.NewResponseController(w)
			if err := rc.SetWriteDeadline(time.Now().Add(30 * time.Second)); err != nil {
				log.Printf("Error setting write deadline: %v", err)
			}
			next.ServeHTTP(w, r)
		})
	}

	v1Router := chi.NewRouter()

	v1Router.Get("/healthz", handler.HandlerReadiness)
	v1Router.Get("/error", handler.HandlerError)

	// routes with 1MB body limit and write timeout
	v1Router.Group(func(r chi.Router) {
		r.Use(limitMiddleware)
		r.Use(writeTimeoutMiddleware)

		r.Post("/user", h.HandlerCreateUser)
		r.Post("/user/login", h.HandlerLogin)
		r.Post("/user/lookup", h.HandlerLookupUser)
		r.Post("/user/exists", h.HandlerEmailExists)
		r.Put("/user", h.MiddlewareAuth(h.HandlerUpdateUser))
		r.Get("/user/me", h.MiddlewareAuth(h.HandlerGetProfile))
		r.Post("/user/refresh", h.HandlerRefreshToken)
		r.Post("/user/logout", h.MiddlewareAuth(h.HandlerLogout))
		r.Post("/user/logout-all", h.MiddlewareAuth(h.HandlerLogoutAll))

		r.Post("/conversations/create", h.MiddlewareAuth(h.HandlerCreateConversation))
		r.Post("/conversations", h.MiddlewareAuth(h.HandlerGetConversations))
		r.Post("/conversations/members", h.MiddlewareAuth(h.HandlerGetConversationMembers))
		r.Post("/conversations/members/add", h.MiddlewareAuth(h.HandlerAddMember))
		r.Post("/conversations/members/remove", h.MiddlewareAuth(h.HandlerRemoveMember))
		r.Post("/conversations/members/role", h.MiddlewareAuth(h.HandlerSetRole))
		r.Post("/conversations/transfer-ownership", h.MiddlewareAuth(h.HandlerTransferOwnership))
		r.Put("/conversations/name", h.MiddlewareAuth(h.HandlerRenameGroup))
		r.Post("/conversations/messages", h.MiddlewareAuth(h.HandlerGetMessages))
		r.Post("/conversations/messages/search", h.MiddlewareAuth(h.HandlerSearchMessages))
		r.Post("/conversations/online", h.MiddlewareAuth(h.HandlerGetOnlineMembers))
		r.Delete("/conversations", h.MiddlewareAuth(h.HandlerDeleteConversation))
	})

	// routes without body limit
	v1Router.Post("/files", h.MiddlewareAuth(h.HandlerUploadFile))
	v1Router.Get("/files/{filename}", h.MiddlewareAuth(h.HandlerServeFile))
	v1Router.Get("/ws", h.HandlerWebSocket(hub, msgHandler))

	router.Mount("/v1", v1Router)

	srv := &http.Server{
		Handler:      router,
		Addr:         ":" + portString,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 0,
		IdleTimeout:  120 * time.Second,
	}

	go func() {
		log.Printf("Server starting on port %v", portString)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal(err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Fatal("Server forced to shutdown:", err)
	}

	hub.Stop()
	log.Println("Server stopped")
}
