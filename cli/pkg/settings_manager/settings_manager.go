package settings_manager

type SettingsManager interface {
	SetUserConfig(config *UserConfig) error
	GetUserConfig() *UserConfig
}
