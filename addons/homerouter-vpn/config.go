package main

import (
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"
	"unicode"
)

// Brand holds everything that is shown to end users, so the same image can be
// re-skinned for another platform without a rebuild.
type Brand struct {
	Name    string `json:"name"`
	Color   string `json:"color"`
	LogoURL string `json:"logoUrl,omitempty"`
}

type Config struct {
	Listen string
	Brand  Brand
}

var colorPattern = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

func getenv(lookup func(string) string, key, fallback string) string {
	if v := strings.TrimSpace(lookup(key)); v != "" {
		return v
	}
	return fallback
}

// LoadConfig reads the VPN_* environment and rejects values that would end up
// unescaped in a page or a style attribute.
func LoadConfig(lookup func(string) string) (Config, error) {
	cfg := Config{
		Listen: getenv(lookup, "VPN_LISTEN", ":8081"),
		Brand: Brand{
			Name:    getenv(lookup, "VPN_BRAND_NAME", "VPN"),
			Color:   getenv(lookup, "VPN_BRAND_COLOR", "#2563eb"),
			LogoURL: getenv(lookup, "VPN_BRAND_LOGO_URL", ""),
		},
	}

	if n := len([]rune(cfg.Brand.Name)); n > 64 {
		return Config{}, errors.New("VPN_BRAND_NAME must be at most 64 characters")
	}
	for _, r := range cfg.Brand.Name {
		if unicode.IsControl(r) {
			return Config{}, errors.New("VPN_BRAND_NAME must not contain control characters")
		}
	}
	if !colorPattern.MatchString(cfg.Brand.Color) {
		return Config{}, fmt.Errorf("VPN_BRAND_COLOR must look like #rrggbb, got %q", cfg.Brand.Color)
	}
	if l := cfg.Brand.LogoURL; l != "" && !strings.HasPrefix(l, "https://") && !strings.HasPrefix(l, "/") {
		return Config{}, errors.New("VPN_BRAND_LOGO_URL must be an https:// URL or an absolute path")
	}
	return cfg, nil
}

func osLookup(key string) string { return os.Getenv(key) }
