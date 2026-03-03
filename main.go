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
	"github.com/Anything-That-Works/GoPath/internal/storage"
	"github.com/Anything-That-Works/GoPath/internal/ws"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/cors"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
)

type apiConfig struct {
	DB           *database.Queries
	JWTSecretKey []byte
	Storage      storage.FileStorage
	Hub          *ws.Hub
	TrustedProxy string
	Cache        cache.Cache
}

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

	apiCfg := apiConfig{
		DB:           database.New(con),
		JWTSecretKey: []byte(jwtSecretKey),
		Storage:      storage.NewLocalStorage(uploadsPath, baseURL),
		Hub:          hub,
		TrustedProxy: trustedProxy,
		Cache:        redisCache,
	}

	msgHandler := ws.NewMessageHandler(hub, apiCfg.DB, apiCfg.Storage)

	router := chi.NewRouter()

	router.Use(cors.Handler(cors.Options{
		AllowedOrigins:   strings.Split(allowedOrigins, ","),
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Authorization", "Content-Type"},
		ExposedHeaders:   []string{"Link"},
		AllowCredentials: false,
		MaxAge:           300,
	}))

	router.Use(middlewareRateLimit)

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

	v1Router.Get("/healthz", handlerReadiness)
	v1Router.Get("/error", handlerError)

	// routes with 1MB body limit and write timeout
	v1Router.Group(func(r chi.Router) {
		r.Use(limitMiddleware)
		r.Use(writeTimeoutMiddleware)

		r.Post("/user", apiCfg.handlerCreateUser)
		r.Post("/user/login", apiCfg.handlerLogin)
		r.Post("/user/lookup", apiCfg.handlerLookupUser)
		r.Post("/user/exists", apiCfg.handlerEmailExists)
		r.Put("/user", apiCfg.middlewareAuth(apiCfg.handlerUpdateUser))
		r.Get("/user/me", apiCfg.middlewareAuth(apiCfg.handlerGetProfile))
		r.Post("/user/refresh", apiCfg.handlerRefreshToken)
		r.Post("/user/logout", apiCfg.middlewareAuth(apiCfg.handlerLogout))
		r.Post("/user/logout-all", apiCfg.middlewareAuth(apiCfg.handlerLogoutAll))

		r.Post("/conversations/create", apiCfg.middlewareAuth(apiCfg.handlerCreateConversation))
		r.Post("/conversations", apiCfg.middlewareAuth(apiCfg.handlerGetConversations))
		r.Post("/conversations/members", apiCfg.middlewareAuth(apiCfg.handlerGetConversationMembers))
		r.Post("/conversations/members/add", apiCfg.middlewareAuth(apiCfg.handlerAddMember))
		r.Post("/conversations/members/remove", apiCfg.middlewareAuth(apiCfg.handlerRemoveMember))
		r.Post("/conversations/members/role", apiCfg.middlewareAuth(apiCfg.handlerSetRole))
		r.Post("/conversations/transfer-ownership", apiCfg.middlewareAuth(apiCfg.handlerTransferOwnership))
		r.Put("/conversations/name", apiCfg.middlewareAuth(apiCfg.handlerRenameGroup))
		r.Post("/conversations/messages", apiCfg.middlewareAuth(apiCfg.handlerGetMessages))
		r.Post("/conversations/messages/search", apiCfg.middlewareAuth(apiCfg.handlerSearchMessages))
		r.Post("/conversations/online", apiCfg.middlewareAuth(apiCfg.handlerGetOnlineMembers))
		r.Delete("/conversations", apiCfg.middlewareAuth(apiCfg.handlerDeleteConversation))
	})

	// routes without body limit
	v1Router.Post("/files", apiCfg.middlewareAuth(apiCfg.handlerUploadFile))
	v1Router.Get("/files/{filename}", apiCfg.middlewareAuth(apiCfg.handlerServeFile))
	v1Router.Get("/ws", apiCfg.handlerWebSocket(hub, msgHandler))

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
