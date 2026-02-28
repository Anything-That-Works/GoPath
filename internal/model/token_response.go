package model

import "time"

type TokenResponse struct {
	AccessToken  string       `json:"access_token"`
	RefreshToken RefreshToken `json:"refresh_token"`
}

type RefreshToken struct {
	Token   string    `json:"token"`
	Created time.Time `json:"created"`
	Expires time.Time `json:"expires"`
}
