package papers

import "testing"

func TestValidateHTTPS_ValidURL(t *testing.T) {
	if err := validateHTTPS("https://export.arxiv.org/api/query"); err != nil {
		t.Errorf("expected nil for https URL, got %v", err)
	}
}

func TestValidateHTTPS_HTTPScheme(t *testing.T) {
	err := validateHTTPS("http://example.com")
	if err == nil {
		t.Error("expected error for http:// URL")
	}
}

func TestValidateHTTPS_FTPScheme(t *testing.T) {
	err := validateHTTPS("ftp://example.com")
	if err == nil {
		t.Error("expected error for ftp:// URL")
	}
}

func TestValidateHTTPS_NoScheme(t *testing.T) {
	err := validateHTTPS("example.com/api")
	if err == nil {
		t.Error("expected error for URL with no scheme")
	}
}
