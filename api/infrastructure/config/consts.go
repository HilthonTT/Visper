package config

const (
	TypeString = "string"
	TypeSelect = "select"
	TypeBool   = "bool"
	TypeText   = "text"
	TypeNumber = "number"

	// preview
	TextTypes          = "text_types"
	AudioTypes         = "audio_types"
	VideoTypes         = "video_types"
	ImageTypes         = "image_types"
	ProxyTypes         = "proxy_types"
	ProxyIgnoreHeaders = "proxy_ignore_headers"
)

const (
	UNKNOWN = iota
	FOLDER
	OFFICE
	VIDEO
	AUDIO
	TEXT
	IMAGE
)

var SlicesMap = make(map[string][]string)
