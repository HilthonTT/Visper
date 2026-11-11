package file

import "time"

type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}

type SuccessResponse struct {
	Message string `json:"message"`
}

type UserResponse struct {
	ID       string `json:"id"`
	Username string `json:"username,omitempty"`
}

type FileResponse struct {
	ID        string       `json:"id"`
	Filename  string       `json:"filename"`
	MimeType  string       `json:"mimetype"`
	Size      int64        `json:"size"`
	URL       string       `json:"url"`
	CreatedAt time.Time    `json:"createdAt"`
	Uploader  UserResponse `json:"uploader"`
}
