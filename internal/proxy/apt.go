package proxy

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"

	log "github.com/sirupsen/logrus"
	"github.com/ubuntu/decorate"
	"golang.org/x/exp/slices"
)

// unsupportedAPTProtocols lists the protocols that are not supported by APT.
var unsupportedAPTProtocols = []protocol{protocolNo, protocolAll, protocolAuto}

// aptString formats a proxy setting to be used in an APT configuration file.
func (p setting) aptString() string {
	if slices.Contains(unsupportedAPTProtocols, p.protocol) {
		log.Debugf("Skipping unsupported APT proxy setting %q", p.protocol)
		return ""
	}
	return fmt.Sprintf("Acquire::%s::Proxy \"%s\";\n", strings.ToLower(p.protocol.String()), p.escapedURL)
}

// applyToAPT applies the proxy configuration in the form of APT settings in /etc/apt/apt.conf.d
// If there are no proxy settings to apply, the APT proxy config file is removed.
func (p Proxy) applyToAPT() (err error) {
	defer decorate.OnError(&err, "couldn't apply apt proxy configuration")

	if p.noSupportedProtocols(unsupportedAPTProtocols) {
		log.Debug("No proxy settings to apply, removing apt proxy config file if it exists")
		if err := os.Remove(p.aptConfigPath); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		return nil
	}

	log.Debugf("Applying APT proxy configuration to %q", p.aptConfigPath)

	content := p.aptConfig()
	if prev, err := previousConfig(p.aptConfigPath); err == nil && prev == content {
		log.Debugf("APT proxy configuration at %q is already up to date", p.aptConfigPath)
		return nil
	} else if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}

	// Check if the parent directory exists - attempt to create the structure if not
	// In practice this is close to impossible because apt itself ships files to
	// this directory, but this simplifies testing a bit for us
	if err := createParentDirectories(p.aptConfigPath); err != nil {
		return err
	}

	return safeWriteFile(p.aptConfigPath, content)
}

// aptConfig returns the formatted APT proxy configuration file to be written.
func (p Proxy) aptConfig() string {
	content := fmt.Sprintln(confHeader)
	for _, p := range p.settings {
		content += p.aptString()
	}

	return content
}
