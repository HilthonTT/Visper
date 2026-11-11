package model

import "time"

type File struct {
	ID        string    `json:"id"`
	RoomID    string    `json:"roomId"`
	UserID    string    `json:"userId"`
	Filename  string    `json:"filename"`
	MimeType  string    `json:"mimeType"`
	Size      int64     `json:"size"`
	Path      string    `json:"path"`
	URL       string    `json:"url"`
	CreatedAt time.Time `json:"createdAt"`
}

func (f *File) IsImage() bool {
	validTypes := map[string]bool{
		"image/jpeg": true,
		"image/jpg":  true,
		"image/png":  true,
		"image/gif":  true,
		"image/webp": true,
	}
	return validTypes[f.MimeType]
}
