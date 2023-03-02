package proxy

import "path/filepath"

// WithRoot overrides the filesystem root for the proxy manager.
func WithRoot(path string) func(o *options) {
	return func(o *options) {
		o.root = path
	}
}

// WithGlibCompileSchemasCmd overrides the glib-compile-schemas command for the proxy manager.
func WithGlibCompileSchemasCmd(cmd []string) func(o *options) {
	return func(o *options) {
		o.glibCompileSchemasCmd = cmd
	}
}

const ConfHeader = confHeader
const DefaultEnvConfigPath = defaultEnvConfigPath
const DefaultAPTConfigPath = defaultAPTConfigPath
const DefaultGLibSchemaPath = defaultGLibSchemaPath

var DefaultGSettingsConfigPath = filepath.Join(defaultGLibSchemaPath, gschemaOverrideFile)
