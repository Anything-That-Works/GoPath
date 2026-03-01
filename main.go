package main

import (
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/Anything-That-Works/GoPath/internal/database"
	"github.com/Anything-That-Works/GoPath/internal/storage"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/cors"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
)

type apiConfig struct {
	DB           *database.Queries
	JWTSecretKey []byte
	Storage      storage.FileStorage
}

func main() {
	envLoadError := godotenv.Load(".env")
	if envLoadError != nil {
		fmt.Println(envLoadError)
	}
	portString := os.Getenv("PORT")

	if portString == "" {
		log.Fatal("PORT value not found.")
	}

	dbURL := os.Getenv("DB_URL")

	if dbURL == "" {
		log.Fatal("DB_URL value not found.")
	}

	jwtSecretKey := os.Getenv("JWT_SECRET_KEY")

	if jwtSecretKey == "" {
		log.Fatal("JWT_SECRET_KEY value not found.")
	}

	con, err := sql.Open("postgres", dbURL)
	if err != nil {
		log.Fatal("Cannot connect to database: ", err)
	}

	apiConfig := apiConfig{
		DB:           database.New(con),
		JWTSecretKey: []byte(jwtSecretKey),
		Storage:      storage.NewLocalStorage("./uploads", "http://localhost:"+portString),
	}

	router := chi.NewRouter()

	router.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"https://*", "http://*"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"*"},
		ExposedHeaders:   []string{"Link"},
		AllowCredentials: false,
		MaxAge:           300,
	}))

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

	v1Router.Post("/conversations/create", apiConfig.middlewareAuth(apiConfig.handlerCreateConversation))
	v1Router.Post("/conversations", apiConfig.middlewareAuth(apiConfig.handlerGetConversations))
	v1Router.Post("/conversations/members", apiConfig.middlewareAuth(apiConfig.handlerGetConversationMembers))
	v1Router.Post("/conversations/members/add", apiConfig.middlewareAuth(apiConfig.handlerAddMember))
	v1Router.Post("/conversations/members/remove", apiConfig.middlewareAuth(apiConfig.handlerRemoveMember))
	v1Router.Post("/conversations/members/role", apiConfig.middlewareAuth(apiConfig.handlerSetRole))
	v1Router.Post("/conversations/transfer-ownership", apiConfig.middlewareAuth(apiConfig.handlerTransferOwnership))
	v1Router.Put("/conversations/name", apiConfig.middlewareAuth(apiConfig.handlerRenameGroup))
	v1Router.Post("/conversations/messages", apiConfig.middlewareAuth(apiConfig.handlerGetMessages))

	v1Router.Post("/files", apiConfig.middlewareAuth(apiConfig.handlerUploadFile))
	v1Router.Get("/files/{filename}", apiConfig.middlewareAuth(apiConfig.handlerServeFile))
	router.Mount("/v1", v1Router)
	srv := &http.Server{
		Handler: router,
		Addr:    ":" + portString,
	}

	log.Printf("Server starting on port %v", portString)

	serveError := srv.ListenAndServe()

	if serveError != nil {
		log.Fatal(serveError)
	}
}
