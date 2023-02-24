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

// envString formats a proxy setting to be environment variable compliant.
func (p setting) envString() string {
	// Return both uppercase and lowercase environment variables for
	// compatibility with different tools
	return fmt.Sprintf("%s_PROXY=%s\n%s_proxy=%s\n",
		strings.ToUpper(fmt.Sprint(p.protocol)), p.escapedURL,
		strings.ToLower(fmt.Sprint(p.protocol)), p.escapedURL)
}

// applyToEnvironment applies the proxy configuration in the form of
// environment variables set in /etc/environment.d.
// If there are no proxy settings to apply, the environment file is removed.
// nolint:dupl // Apply logic is similar to applyToApt
func (p Proxy) applyToEnvironment() (err error) {
	defer decorate.OnError(&err, "couldn't apply environment proxy configuration")

	if len(p.settings) == 0 {
		log.Debug("No proxy settings to apply, removing environment file if it exists")
		if err := os.Remove(p.envConfigPath); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		return nil
	}

	log.Debugf("Applying environment proxy configuration to %q", p.envConfigPath)
	content := fmt.Sprintln(confHeader)
	for _, p := range p.settings {
		content += p.envString()
	}

	if prevContent, err := previousConfig(p.envConfigPath); err == nil && prevContent == content {
		log.Debugf("Environment proxy configuration at %q is already up to date", p.envConfigPath)
		return nil
	} else if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}

	// Check if the parent directory exists - attempt to create the structure if not
	envConfigDir := filepath.Dir(p.envConfigPath)
	if _, err := os.Stat(envConfigDir); errors.Is(err, fs.ErrNotExist) {
		log.Debugf("Creating directory %q", envConfigDir)
		// #nosec G301 - /etc/environment.d permissions are 0755, so we should keep the same pattern
		if err := os.MkdirAll(envConfigDir, 0755); err != nil {
			return fmt.Errorf("failed to create environment config parent directory: %w", err)
		}
	} else if err != nil {
		return fmt.Errorf("unexpected error while checking environment directory: %w", err)
	}

	// #nosec G306 - /etc/environment.d/* permissions are 0644, so we should keep the same pattern
	if err := os.WriteFile(p.envConfigPath+".new", []byte(content), 0644); err != nil {
		return err
	}
	if err := os.Rename(p.envConfigPath+".new", p.envConfigPath); err != nil {
		return err
	}

	return nil
}
