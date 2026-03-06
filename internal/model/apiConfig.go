package model

import (
	"github.com/Anything-That-Works/GoPath/internal/cache"
	"github.com/Anything-That-Works/GoPath/internal/database"
	"github.com/Anything-That-Works/GoPath/internal/storage"
	"github.com/Anything-That-Works/GoPath/internal/ws"
)

type ApiConfig struct {
	DB           *database.Queries
	JWTSecretKey []byte
	Storage      storage.FileStorage
	Hub          *ws.Hub
	TrustedProxy string
	Cache        cache.Cache
}
