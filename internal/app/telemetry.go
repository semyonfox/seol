package app

import (
	"errors"
	"net/url"
	"strings"
	"unicode"
)

func validateTelemetryConfig(cfg Config) error {
	if !cfg.TelemetryEnabled {
		return nil
	}
	endpoint := cfg.TelemetryEndpoint
	parsed, err := url.Parse(endpoint)
	invalid := err != nil || endpoint == "" || strings.HasPrefix(endpoint, "//") ||
		strings.ContainsAny(endpoint, "\\?#") || strings.ContainsFunc(endpoint, unicode.IsSpace)
	if !invalid {
		invalid = parsed.User != nil || parsed.Opaque != "" ||
			!(parsed.Scheme == "https" && parsed.Hostname() != "" ||
				strings.HasPrefix(endpoint, "/") && parsed.Scheme == "" && parsed.Host == "")
	}
	if invalid {
		return errors.New("telemetry requires an HTTPS endpoint or a relative proxy path, without credentials, query or fragment")
	}
	return nil
}
