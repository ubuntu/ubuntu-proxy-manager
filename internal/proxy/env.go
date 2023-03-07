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

// unsupportedEnvProtocols lists protocols that are not supported by the environment proxy.
var unsupportedEnvProtocols = []protocol{protocolAuto}

// envString formats a proxy setting to be environment variable compliant.
func (p setting) envString() string {
	if slices.Contains(unsupportedEnvProtocols, p.protocol) {
		log.Debugf("Skipping unsupported environment proxy setting %q", p.protocol)
		return ""
	}

	value := p.escapedURL
	// Trim unwanted characters for no_proxy
	if p.protocol == protocolNo {
		value = strings.NewReplacer(" ", "", "'", "", `"`, "").Replace(value)
	}

	// Return both uppercase and lowercase environment variables for
	// compatibility with different tools
	return fmt.Sprintf("%s_PROXY=%q\n%s_proxy=%q\n",
		strings.ToUpper(fmt.Sprint(p.protocol)), value,
		strings.ToLower(fmt.Sprint(p.protocol)), value)
}

// applyToEnvironment applies the proxy configuration in the form of
// environment variables set in /etc/environment.d.
// If there are no proxy settings to apply, the environment file is removed.
func (p Proxy) applyToEnvironment() (err error) {
	defer decorate.OnError(&err, "couldn't apply environment proxy configuration")

	if p.noSupportedProtocols(unsupportedEnvProtocols) {
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
