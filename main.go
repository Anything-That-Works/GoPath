package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

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
}

func requireEnv(key string) string {
	val := os.Getenv(key)
	if val == "" {
		log.Fatalf("%s value not found.", key)
	}
	return val
}

func main() {
	// load .env first before calling requireEnv
	if err := godotenv.Load(".env"); err != nil {
		fmt.Println(err)
	}

	portString := requireEnv("PORT")
	dbURL := requireEnv("DB_URL")
	jwtSecretKey := requireEnv("JWT_SECRET_KEY")
	allowedOrigins := requireEnv("ALLOWED_ORIGINS")
	uploadsPath := requireEnv("UPLOADS_PATH")
	baseURL := requireEnv("BASE_URL")

	con, err := sql.Open("postgres", dbURL)
	if err != nil {
		log.Fatal("Cannot connect to database: ", err)
	}

	con.SetMaxOpenConns(25)
	con.SetMaxIdleConns(10)
	con.SetConnMaxLifetime(5 * time.Minute)
	con.SetConnMaxIdleTime(2 * time.Minute)

	hub := ws.NewHub()
	go hub.Run()

	apiConfig := apiConfig{
		DB:           database.New(con),
		JWTSecretKey: []byte(jwtSecretKey),
		Storage:      storage.NewLocalStorage(uploadsPath, baseURL),
		Hub:          hub,
	}

	msgHandler := ws.NewMessageHandler(hub, apiConfig.DB, apiConfig.Storage)

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
	router.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r.Body = http.MaxBytesReader(w, r.Body, 1*1024*1024)
			next.ServeHTTP(w, r)
		})
	})
	router.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Content-Type-Options", "nosniff")
			w.Header().Set("X-Frame-Options", "DENY")
			w.Header().Set("X-XSS-Protection", "1; mode=block")
			w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
			next.ServeHTTP(w, r)
		})
	})

	v1Router := chi.NewRouter()
	v1Router.Get("/healthz", handlerReadiness)
	v1Router.Get("/error", handlerError)
	v1Router.Post("/user", apiConfig.handlerCreateUser)
	v1Router.Post("/user/login", apiConfig.handlerLogin)
	v1Router.Post("/user/lookup", apiConfig.handlerLookupUser)
	v1Router.Post("/user/exists", apiConfig.handlerEmailExists)
	v1Router.Put("/user", apiConfig.middlewareAuth(apiConfig.handlerUpdateUser))
	v1Router.Get("/user/me", apiConfig.middlewareAuth(apiConfig.handlerGetProfile))
	v1Router.Post("/user/refresh", apiConfig.handlerRefreshToken)
	v1Router.Post("/user/logout", apiConfig.middlewareAuth(apiConfig.handlerLogout))
	v1Router.Post("/user/logout-all", apiConfig.middlewareAuth(apiConfig.handlerLogoutAll))

	v1Router.Post("/conversations/create", apiConfig.middlewareAuth(apiConfig.handlerCreateConversation))
	v1Router.Post("/conversations", apiConfig.middlewareAuth(apiConfig.handlerGetConversations))
	v1Router.Post("/conversations/members", apiConfig.middlewareAuth(apiConfig.handlerGetConversationMembers))
	v1Router.Post("/conversations/members/add", apiConfig.middlewareAuth(apiConfig.handlerAddMember))
	v1Router.Post("/conversations/members/remove", apiConfig.middlewareAuth(apiConfig.handlerRemoveMember))
	v1Router.Post("/conversations/members/role", apiConfig.middlewareAuth(apiConfig.handlerSetRole))
	v1Router.Post("/conversations/transfer-ownership", apiConfig.middlewareAuth(apiConfig.handlerTransferOwnership))
	v1Router.Put("/conversations/name", apiConfig.middlewareAuth(apiConfig.handlerRenameGroup))
	v1Router.Post("/conversations/messages", apiConfig.middlewareAuth(apiConfig.handlerGetMessages))
	v1Router.Post("/conversations/messages/search", apiConfig.middlewareAuth(apiConfig.handlerSearchMessages))
	v1Router.Post("/conversations/online", apiConfig.middlewareAuth(apiConfig.handlerGetOnlineMembers))
	v1Router.Delete("/conversations", apiConfig.middlewareAuth(apiConfig.handlerDeleteConversation))

	v1Router.Post("/files", apiConfig.middlewareAuth(apiConfig.handlerUploadFile))
	v1Router.Get("/files/{filename}", apiConfig.middlewareAuth(apiConfig.handlerServeFile))

	v1Router.Get("/ws", apiConfig.handlerWebSocket(hub, msgHandler))

	router.Mount("/v1", v1Router)

	srv := &http.Server{
		Handler:      router,
		Addr:         ":" + portString,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
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

	log.Println("Server stopped")
}
