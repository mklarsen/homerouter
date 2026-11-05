package main

import (
	"errors"
	"fmt"
	"net"
	"net/netip"
	"os"
	"regexp"
	"strconv"
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

	// APIToken enables the peer API when set. Without it the API is not served.
	APIToken   string
	Backend    string // "fake" or "wg"
	Iface      string
	Pool       *Pool
	Port       int
	Endpoint   string
	DNS        string
	AllowedIPs []string
	DataDir    string

	// OIDC is nil unless VPN_OIDC_ISSUER is set.
	OIDC *OIDCConfig
}

var (
	colorPattern = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)
	ifacePattern = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_-]{0,14}$`)
)

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

	if err := loadVPN(lookup, &cfg); err != nil {
		return Config{}, err
	}
	if err := loadOIDC(lookup, &cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func loadVPN(lookup func(string) string, cfg *Config) error {
	cfg.APIToken = getenv(lookup, "VPN_API_TOKEN", "")
	if cfg.APIToken != "" && len(cfg.APIToken) < 32 {
		return errors.New("VPN_API_TOKEN must be at least 32 characters")
	}

	cfg.Backend = getenv(lookup, "VPN_BACKEND", "fake")
	if cfg.Backend != "fake" && cfg.Backend != "wg" {
		return fmt.Errorf("VPN_BACKEND must be fake or wg, got %q", cfg.Backend)
	}

	cfg.Iface = getenv(lookup, "VPN_IF", "wg0")
	if !ifacePattern.MatchString(cfg.Iface) {
		return fmt.Errorf("VPN_IF %q is not a valid interface name", cfg.Iface)
	}

	var err error
	if cfg.Pool, err = NewPool(getenv(lookup, "VPN_NET4", "10.10.30.0/24")); err != nil {
		return fmt.Errorf("VPN_NET4: %w", err)
	}

	portStr := getenv(lookup, "VPN_PORT", "51820")
	if cfg.Port, err = strconv.Atoi(portStr); err != nil || cfg.Port < 1 || cfg.Port > 65535 {
		return fmt.Errorf("VPN_PORT must be 1-65535, got %q", portStr)
	}

	cfg.Endpoint = getenv(lookup, "VPN_ENDPOINT", "")
	if cfg.Endpoint == "" {
		if cfg.Backend == "wg" {
			return errors.New("VPN_ENDPOINT (host:port) is required with VPN_BACKEND=wg")
		}
		cfg.Endpoint = "vpn.example.invalid:" + portStr
	}
	host, endPort, err := net.SplitHostPort(cfg.Endpoint)
	if err != nil || host == "" {
		return fmt.Errorf("VPN_ENDPOINT must be host:port, got %q", cfg.Endpoint)
	}
	if n, err := strconv.Atoi(endPort); err != nil || n < 1 || n > 65535 {
		return fmt.Errorf("VPN_ENDPOINT has an invalid port: %q", cfg.Endpoint)
	}

	if cfg.DNS = getenv(lookup, "VPN_DNS", ""); cfg.DNS != "" {
		if _, err := netip.ParseAddr(cfg.DNS); err != nil {
			return fmt.Errorf("VPN_DNS must be an IP address: %w", err)
		}
	}

	allowed := getenv(lookup, "VPN_ALLOWED_IPS", cfg.Pool.Prefix().String())
	for _, p := range strings.Split(allowed, ",") {
		p = strings.TrimSpace(p)
		if _, err := netip.ParsePrefix(p); err != nil {
			return fmt.Errorf("VPN_ALLOWED_IPS entry %q: %w", p, err)
		}
		cfg.AllowedIPs = append(cfg.AllowedIPs, p)
	}

	cfg.DataDir = getenv(lookup, "VPN_DATA_DIR", "/data")
	return nil
}

func osLookup(key string) string { return os.Getenv(key) }
