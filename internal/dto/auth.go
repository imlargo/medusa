package dto

import "github.com/imlargo/go-api/internal/models"

type AuthResponse struct {
	User   models.User `json:"user"`
	Tokens AuthTokens  `json:"tokens"`
}

type AuthTokens struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
}
