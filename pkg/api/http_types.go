package api

// Endpoint: /auth/me
type AccountSession struct {
	Name      string `json:"name"` // Public identifier for session management
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

// Endpoint: /storage/list
type SingleObject struct {
	Key       string            `json:"key,omitempty"`
	Path      string            `json:"path"`
	Password  string            `json:"password,omitempty"`
	CreatedAt int64             `json:"created_at"`
	Meta      map[string]string `json:"meta,omitempty"`
}
type ListResponse []SingleObject

// Endpoint: /storage/upload
type UploadResponse struct {
	Uid  string `json:"uid"`
	Path string `json:"path"`
}

// Endpoint: /storage/remove
type RemoveResponse struct {
	Results map[string]string `json:"results"`
}

// Endpoint: /storage/info
type InspectResponse struct {
	Key       string         `json:"key"`
	Owner     string         `json:"owner"`
	Path      string         `json:"path"`
	CreatedAt int64          `json:"created_at"`
	Protected bool           `json:"protected"`
	Metadata  map[string]any `json:"metadata,omitempty"`
}
