package papers

import (
	"fmt"
	"net/url"
)

// validateHTTPS returns an error if the URL scheme is not https.
func validateHTTPS(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("parse URL: %w", err)
	}
	if u.Scheme != "https" {
		return fmt.Errorf("URL must use https, got %q", u.Scheme)
	}
	return nil
}
