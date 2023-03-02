package proxy

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	log "github.com/sirupsen/logrus"
	"github.com/ubuntu/decorate"
)

const (
	// systemProxySchemaID is the GSettings schema ID for system proxy configuration.
	systemProxySchemaID = "org.gnome.system.proxy"
)

// gsettingsString formats a proxy setting to be used in a GSchema override file.
func (p setting) gsettingsString() string {
	var section, settings string

	switch p.protocol {
	case protocolHTTP, protocolHTTPS, protocolFTP, protocolSOCKS:
		section = fmt.Sprintf("[%s.%s]", systemProxySchemaID, strings.ToLower(p.protocol.String()))
		settings = fmt.Sprintf("host='%s'\n", p.url.Hostname())
		if p.url.Port() != "" {
			settings += fmt.Sprintf("port=%s\n", p.url.Port())
		}

		// Authentication is only supported for HTTP proxy
		if p.url.User != nil {
			if p.protocol != protocolHTTP {
				log.Warningf("GSettings authentication is only supported for HTTP proxy, ignoring for %s", p.protocol)
				break
			}
			settings += "use-authentication=true\n"
			settings += fmt.Sprintf("authentication-user='%s'\n", escapeSingleQuote(p.url.User.Username()))
			if password, ok := p.url.User.Password(); ok {
				settings += fmt.Sprintf("authentication-password='%s'\n", escapeSingleQuote(password))
			}
		}
	case protocolNo:
		// Ignored hosts are configured at the root level
		section = fmt.Sprintf("[%s]", systemProxySchemaID)

		hosts := strings.Split(p.escapedURL, ",")
		for i, host := range hosts {
			hosts[i] = wrapHostIfNeeded(host)
		}
		settings = fmt.Sprintf("ignore-hosts=[%s]\n", strings.Join(hosts, ","))
	case protocolAuto:
		// Autoconfig URL is configured at the root level
		section = fmt.Sprintf("[%s]", systemProxySchemaID)
		settings = fmt.Sprintf("autoconfig-url='%s'\n", p.escapedURL)
	}

	return fmt.Sprintf("%s\n%s\n", section, settings)
}

// applyToGSettings applies the proxy configuration in the form of a GSchema override file,
// then runs glib-compile-schemas to make the changes visible to GSettings.
// If there are no proxy settings to apply, the GSchema override file is removed.
func (p Proxy) applyToGSettings() (err error) {
	defer decorate.OnError(&err, "couldn't apply GSettings proxy configuration")

	// On the off chance that the user is not running GNOME, we want to print a warning and quietly return.
	if _, err := exec.LookPath(p.glibCompileSchemasCmd[0]); err != nil {
		log.Warningf("Couldn't find an executable for %q, not applying GSettings proxy configuration", p.glibCompileSchemasCmd[0])
		return nil
	}

	// Check if the parent directory exists - fail if it doesn't, as it means we
	// don't have any defined proxy XML schema to override.
	if stat, err := os.Stat(p.glibSchemasPath); err != nil {
		return fmt.Errorf("couldn't find GLib schema directory: %w", err)
	} else if !stat.IsDir() {
		return fmt.Errorf("GLib schema path %q is not a directory", filepath.Dir(p.gsettingsConfigPath))
	}

	if len(p.settings) == 0 {
		log.Debug("No proxy settings to apply, removing GSchema override file if it exists")

		// If we managed to remove something, we need to recompile the schemas
		// to propagate the change to GSettings.
		if err := os.Remove(p.gsettingsConfigPath); err == nil {
			log.Debugf("Removed GSettings override file at %q", p.gsettingsConfigPath)
			return p.runGlibCompileSchemas()
		} else if !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		return nil
	}
	log.Debugf("Applying GSettings proxy configuration to %q", p.gsettingsConfigPath)

	content := p.gsettingsConfig()
	prevContent, err := previousConfig(p.gsettingsConfigPath)
	if err == nil && prevContent == content {
		log.Debugf("GSettings proxy configuration at %q is already up to date", p.gsettingsConfigPath)
		return nil
	} else if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}

	if err := safeWriteFile(p.gsettingsConfigPath, content); err != nil {
		return err
	}

	if err := p.runGlibCompileSchemas(); err != nil {
		// If we failed to recompile the schemas (due to our fault or not),
		// revert to the previous version of the configuration file.
		return errors.Join(err, safeWriteFile(p.gsettingsConfigPath, prevContent))
	}

	return nil
}

// gsettingsConfig returns the formatted GSettings proxy configuration file to be written.
func (p Proxy) gsettingsConfig() string {
	content := fmt.Sprintln(confHeader)
	for _, p := range p.settings {
		content += p.gsettingsString()
	}
	content += fmt.Sprintf("[%s]\n", systemProxySchemaID)
	content += fmt.Sprintf("mode='%s'\n", p.gsettingsProxyMode())

	return content
}

// gsettingsProxyMode returns the GSettings proxy mode to be used.
// If an autoconfig URL is set, auto is returned.
// If only specific protocols are set, manual is returned.
func (p Proxy) gsettingsProxyMode() string {
	for _, setting := range p.settings {
		if setting.protocol == protocolAuto {
			return "auto"
		}
	}

	return "manual"
}

// runGlibCompileSchemas runs glib-compile-schemas on the default GSettings schema path.
func (p Proxy) runGlibCompileSchemas() error {
	glibCompileSchemasCmd := append(p.glibCompileSchemasCmd, "--strict", p.glibSchemasPath)
	log.Debugf("Running glib-compile-schemas on %q", p.glibSchemasPath)

	// #nosec G204 - path not controllable by user
	out, err := exec.Command(glibCompileSchemasCmd[0], glibCompileSchemasCmd[1:]...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("couldn't run glib-compile-schemas: %w: %s", err, out)
	}
	if len(out) > 0 {
		log.Debugf("glib-compile-schemas output: %s", out)
	}

	return nil
}

// wrapHostIfNeeded wraps the host in single quotes if it is not already wrapped.
func wrapHostIfNeeded(host string) string {
	trimmedHost := strings.Trim(host, `'"`)

	return fmt.Sprintf("'%s'", trimmedHost)
}

// escapeSingleQuote escapes single quotes in the given string.
func escapeSingleQuote(s string) string {
	return strings.ReplaceAll(s, "'", `\'`)
}
