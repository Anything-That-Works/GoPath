package main

import (
	"github.com/Anything-That-Works/GoPath/internal/database"
	"github.com/Anything-That-Works/GoPath/internal/model"
)

func databaseUserToUserSummary(dbUser database.User) model.UserSummary {
	var name *string
	if dbUser.Name.Valid {
		name = &dbUser.Name.String
	}
	return model.UserSummary{
		ID:    dbUser.ID.String(),
		Name:  name,
		Email: dbUser.Email,
	}
}
