package embeds

import (
	"embed"
)

//go:embed waifu.png
var WaifuImage []byte

//go:embed waifu2.png
var Waifu2Image []byte

//go:embed faq.json
var JsonFaqData embed.FS

//go:embed waifus.json
var WaifusData embed.FS
