package sdk

type Config interface {
	GetString(key string) string
	GetInt(key string) int
	GetBool(key string) bool
	GetStringSlice(key string) []string
	Sub(key string) Config
}
