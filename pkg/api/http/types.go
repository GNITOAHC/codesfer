package apihttp

import "codesfer/pkg/object"

// Endpoint: /auth/me
type AccountSession struct {
	Location  string `json:"location"`
	Agent     string `json:"agent"`
	LastSeen  string `json:"last_seen"`
	CreatedAt string `json:"created_at"`
	Current   bool   `json:"current"`
}
type AccountResponse struct {
	Email    string           `json:"email"`
	Username string           `json:"username"`
	Sessions []AccountSession `json:"sessions"`
}

// Endpoint: /auth/register
type RegisterRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Username string `json:"username"`
}
type RegisterResponse string

// Endpoint: /list
type ListResponse []object.Object
