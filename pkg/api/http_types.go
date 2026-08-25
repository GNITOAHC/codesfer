package api

// Endpoint: /auth/me
type AccountSession struct {
	Name      string `json:"name"` // Public identifier for session management
	Location  string `json:"location"`
	Agent     string `json:"agent"`
	LastSeen  int64  `json:"last_seen"`
	CreatedAt int64  `json:"created_at"`
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
	Key         string            `json:"key,omitempty"`
	Path        string            `json:"path"`
	Password    string            `json:"password,omitempty"`
	CreatedAt   int64             `json:"created_at"`
	Meta        map[string]string `json:"meta,omitempty"`
	AccessScope string            `json:"access_scope"` // owner | authenticated | public
}
type ListResponse []SingleObject

// Endpoint: /storage/upload
type UploadResponse struct {
	Uid  string `json:"uid"`
	Path string `json:"path"`
}

// Endpoint: /storage/remove
// Removes one key per request; returns 204 No Content on success.

// Endpoint: /storage/settings
// Nil fields are left unchanged; desc set to "" removes the description.
type UpdateSettingsRequest struct {
	Key         *string `json:"key,omitempty"`      // new id
	IdxPath     *string `json:"idx_path,omitempty"` // new idxPath (the `path` shown by list/inspect)
	Desc        *string `json:"desc,omitempty"`     // metadata description
	AccessScope *string `json:"access_scope,omitempty"`
}

// Endpoint: /storage/info
type InspectResponse struct {
	Key         string         `json:"key"`
	Owner       string         `json:"owner"`
	Path        string         `json:"path"`
	CreatedAt   int64          `json:"created_at"`
	Protected   bool           `json:"protected"`    // true if password-protected
	AccessScope string         `json:"access_scope"` // owner | authenticated | public
	Metadata    map[string]any `json:"metadata,omitempty"`
}
