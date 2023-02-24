package proxy

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	log "github.com/sirupsen/logrus"
	"github.com/ubuntu/decorate"
)

// aptString formats a proxy setting to be used in an APT configuration file.
func (p setting) aptString() string {
	// no_proxy is not supported by APT, so we ignore it
	if p.protocol == protocolNo {
		return ""
	}
	return fmt.Sprintf("Acquire::%s::Proxy \"%s\";\n", strings.ToLower(p.protocol.String()), p.escapedURL)
}

// applyToApt applies the proxy configuration in the form of APT settings in /etc/apt/apt.conf.d
// If there are no proxy settings to apply, the APT proxy config file is removed.
// nolint:dupl // Apply logic is similar to applyToEnvironment
func (p Proxy) applyToApt() (err error) {
	defer decorate.OnError(&err, "couldn't apply apt proxy configuration")

	if len(p.settings) == 0 {
		log.Debug("No proxy settings to apply, removing apt proxy config file if it exists")
		if err := os.Remove(p.aptConfigPath); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		return nil
	}

	log.Debugf("Applying APT proxy configuration to %q", p.aptConfigPath)
	content := fmt.Sprintln(confHeader)
	for _, p := range p.settings {
		content += p.aptString()
	}

	if prevContent, err := previousConfig(p.aptConfigPath); err == nil && prevContent == content {
		log.Debugf("APT proxy configuration at %q is already up to date", p.aptConfigPath)
		return nil
	} else if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}

	// Check if the parent directory exists - attempt to create the structure if not
	// In practice this is close to impossible because apt itself ships files to
	// this directory, but this simplifies testing a bit for us
	aptConfigDir := filepath.Dir(p.aptConfigPath)
	if _, err := os.Stat(aptConfigDir); errors.Is(err, fs.ErrNotExist) {
		log.Debugf("Creating directory %q", aptConfigDir)
		// #nosec G301 - /etc/apt/apt.conf.d permissions are 0755, so we should keep the same pattern
		if err := os.MkdirAll(aptConfigDir, 0755); err != nil {
			return fmt.Errorf("failed to create APT config directory: %w", err)
		}
	} else if err != nil {
		return fmt.Errorf("unexpected error while checking APT config directory: %w", err)
	}

	// #nosec G306 - /etc/apt/apt.conf.d/* permissions are 0644, so we should keep the same pattern
	if err := os.WriteFile(p.aptConfigPath+".new", []byte(content), 0644); err != nil {
		return err
	}
	if err := os.Rename(p.aptConfigPath+".new", p.aptConfigPath); err != nil {
		return err
	}

	return nil
}
