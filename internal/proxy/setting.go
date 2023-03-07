package proxy

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/ubuntu/decorate"
	"golang.org/x/exp/slices"
)

//go:generate stringer -type=protocol -trimprefix=protocol
type protocol uint8

const (
	protocolAll protocol = iota
	protocolNo
	protocolHTTP
	protocolHTTPS
	protocolFTP
	protocolSOCKS
	protocolAuto // autoconfiguration URL
)

// setting represents a proxy setting to be applied on the system.
type setting struct {
	protocol   protocol
	escapedURL string // scheme://host:port, including escaped user:password if available, verbatim if no_proxy

	url *url.URL
}

// newSettings parses and validates the given proxy settings, returning them in a
// format ready to be applied on the system.
func newSettings(http, https, ftp, socks, noproxy, auto string) (settings []setting, err error) {
	defer decorate.OnError(&err, "couldn't set proxy configuration")

	if http != "" {
		setting, err := newSetting(protocolHTTP, http)
		if err != nil {
			return nil, err
		}
		settings = append(settings, setting)
	}

	if https != "" {
		setting, err := newSetting(protocolHTTPS, https)
		if err != nil {
			return nil, err
		}
		settings = append(settings, setting)
	}

	if ftp != "" {
		setting, err := newSetting(protocolFTP, ftp)
		if err != nil {
			return nil, err
		}
		settings = append(settings, setting)
	}

	if socks != "" {
		setting, err := newSetting(protocolSOCKS, socks)
		if err != nil {
			return nil, err
		}
		settings = append(settings, setting)
	}

	// If all protocols are set, and the escaped values are the same, we can safely set all_proxy as well
	if len(settings) == 4 && slices.EqualFunc(settings, settings, func(a, _ setting) bool { return a.escapedURL == settings[0].escapedURL }) {
		setting, err := newSetting(protocolAll, http)
		if err != nil {
			return nil, err
		}
		settings = append(settings, setting)
	}

	if noproxy != "" {
		setting, err := newSetting(protocolNo, noproxy)
		if err != nil {
			return nil, err
		}
		settings = append(settings, setting)
	}

	if auto != "" {
		setting, err := newSetting(protocolAuto, auto)
		if err != nil {
			return nil, err
		}
		settings = append(settings, setting)
	}

	return settings, nil
}

// newSetting creates a new proxy setting from the given protocol and URL.
// It returns an error if the URL is invalid.
func newSetting(proto protocol, uri string) (p setting, err error) {
	defer decorate.OnError(&err, "couldn't create proxy setting")

	// Autoconfiguration URLs and noProxy are special cases which we don't need to parse
	if slices.Contains([]protocol{protocolNo, protocolAuto}, proto) {
		return setting{protocol: proto, escapedURL: uri}, nil
	}
	// Ideally we would've handled this after calling url.Parse, by checking the
	// Scheme attribute, but it's not reliable in case we parse an URI like
	// "example.com:8000" and "example.com" is treated as a scheme because of
	// the colon in the URI.
	if !strings.Contains(uri, "://") {
		return p, fmt.Errorf("missing scheme in proxy URI %q", uri)
	}

	uri = escapeURLCredentials(uri)
	parsedURL, err := url.Parse(uri)
	if err != nil {
		return p, err
	}

	var host string
	if parsedURL.User != nil {
		host = parsedURL.User.String() + "@"
	}
	host += parsedURL.Host
	escapedURL := fmt.Sprintf("%s://%s", parsedURL.Scheme, host)

	return setting{
		escapedURL: escapedURL,
		protocol:   proto,
		url:        parsedURL,
	}, nil
}

// escapeURLCredentials escapes special characters from the credentials in the
// given URL, if any.
func escapeURLCredentials(uri string) string {
	// Attempt to unescape the string first, discarding any error
	// At best, this prevents us from escaping the URL multiple times
	// At worst, the URL is not affected (we will treat % signs as part of the
	// credentials and escape them later)
	uri, _ = url.PathUnescape(uri)

	// Regexp to check if the URI contains credentials
	r := regexp.MustCompile(`^\w+://(?:(?P<credentials>.*:?.*)@)[a-zA-Z0-9.-]+(:[0-9]+)?/?$`)
	matchIndex := r.SubexpIndex("credentials")
	matches := r.FindStringSubmatch(uri)
	if len(matches) >= matchIndex {
		creds := matches[matchIndex]
		user, password, found := strings.Cut(creds, ":")
		if found {
			return strings.Replace(uri, creds, url.UserPassword(user, password).String(), 1)
		}
		return strings.Replace(uri, creds, url.PathEscape(user), 1)
	}

	return uri
}
