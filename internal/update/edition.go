package update

import (
	"fmt"
	"strings"
)

type editionConfig struct {
	Edition           string
	ServiceName       string
	BinaryName        string
	UpdateCheckURL    string
	GitHubReleasesAPI string
	GitHubTagPrefix   string
}

func newEditionConfig(edition string) editionConfig {
	if strings.EqualFold(strings.TrimSpace(edition), "lite") {
		return editionConfig{
			Edition:           "lite",
			ServiceName:       "clawpanel-lite",
			BinaryName:        "clawpanel-lite",
			UpdateCheckURL:    "http://39.102.53.188:16198/clawpanel/update-lite.json",
			GitHubReleasesAPI: "https://api.github.com/repos/zhaoxinyi02/ClawPanel/releases?per_page=20",
			GitHubTagPrefix:   "lite-v",
		}
	}
	return editionConfig{
		Edition:           "pro",
		ServiceName:       "clawpanel",
		BinaryName:        "clawpanel",
		UpdateCheckURL:    "http://39.102.53.188:16198/clawpanel/update-pro.json",
		GitHubReleasesAPI: "https://api.github.com/repos/zhaoxinyi02/ClawPanel/releases?per_page=20",
		GitHubTagPrefix:   "pro-v",
	}
}

func (c editionConfig) matchesTag(tag string) bool {
	return strings.HasPrefix(strings.TrimSpace(tag), c.GitHubTagPrefix)
}

func (c editionConfig) trimTag(tag string) string {
	return strings.TrimPrefix(strings.TrimSpace(tag), c.GitHubTagPrefix)
}

func (c editionConfig) assetPrefix(version string) string {
	if c.Edition == "lite" {
		return fmt.Sprintf("clawpanel-lite-core-v%s", version)
	}
	return fmt.Sprintf("clawpanel-v%s", version)
}
