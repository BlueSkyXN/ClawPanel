package buildinfo

var (
	Version = "dev"
	Edition = "pro"
)

func NormalizedEdition() string {
	switch Edition {
	case "lite":
		return "lite"
	default:
		return "pro"
	}
}

func IsLite() bool {
	return NormalizedEdition() == "lite"
}
