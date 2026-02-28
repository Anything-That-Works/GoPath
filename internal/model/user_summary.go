package model

type UserSummary struct {
	ID    string  `json:"id"`
	Name  *string `json:"name"`
	Email string  `json:"email"`
}
