// Package settings persists user preferences for the visper CLI.
package settings

// Manager loads and stores the user's configuration.
type Manager interface {
	SetUserConfig(config *UserConfig) error
	GetUserConfig() *UserConfig
}
