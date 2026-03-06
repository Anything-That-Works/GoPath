package model

import (
	"github.com/Anything-That-Works/GoPath/internal/database"
)

type UserSummary struct {
	ID    string  `json:"id"`
	Name  *string `json:"name"`
	Email string  `json:"email"`
}

func DatabaseUserToUserSummary(dbUser database.User) UserSummary {
	var name *string
	if dbUser.Name.Valid {
		name = &dbUser.Name.String
	}
	return UserSummary{
		ID:    dbUser.ID.String(),
		Name:  name,
		Email: dbUser.Email,
	}
}
