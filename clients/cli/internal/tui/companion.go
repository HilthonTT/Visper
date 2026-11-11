package tui

import (
	"encoding/json"
	"log"

	"github.com/hilthontt/visper/clients/cli/internal/tui/embeds"
)

// The companion is the picture shown in the chat's side panel, chosen on the
// settings page.

// secondCompanionID picks the second embedded image; any other ID (as stored
// in the user config and listed in waifus.json) shows the first.
const secondCompanionID = 2

type companion struct {
	ID          int    `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
}

// loadCompanions reads the embedded companion list; it is part of the binary,
// so a failure is a build error and fatal.
func loadCompanions() []companion {
	data, err := embeds.WaifusData.ReadFile("waifus.json")
	if err != nil {
		log.Fatalf("Failed to read embedded file: %s", err)
	}
	var companions []companion
	if err := json.Unmarshal(data, &companions); err != nil {
		log.Fatalf("Failed to unmarshal JSON: %s", err)
	}
	return companions
}

func (m model) selectedCompanionID() int {
	return m.settingsManager.GetUserConfig().SelectedWaifu
}

// companionImage renders a companion's picture at the given size. It decodes
// and resizes a PNG, so call it from Update and cache the result.
func (m model) companionImage(id, width, height int) string {
	img := embeds.WaifuImage
	if id == secondCompanionID {
		img = embeds.Waifu2Image
	}
	content, err := m.imagePreviewer.ImagePreviewFromBytes(img, width, height, "")
	if err != nil {
		log.Printf("Failed to load embedded image: %v", err)
		return m.theme.TextSubtle().Render("image unavailable")
	}
	return content
}
