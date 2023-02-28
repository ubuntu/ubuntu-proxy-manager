package proxy

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
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

	content := p.envConfig()
	if prev, err := previousConfig(p.envConfigPath); err == nil && prev == content {
		log.Debugf("Environment proxy configuration at %q is already up to date", p.envConfigPath)
		return nil
	} else if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}

	// Check if the parent directory exists - attempt to create the structure if not
	if err := createParentDirectories(p.envConfigPath); err != nil {
		return err
	}

	return safeWriteFile(p.envConfigPath, content)
}

// envConfig returns the formatted environment proxy configuration file to be written.
func (p Proxy) envConfig() string {
	content := fmt.Sprintln(confHeader)
	for _, p := range p.settings {
		content += p.envString()
	}

	return content
}
