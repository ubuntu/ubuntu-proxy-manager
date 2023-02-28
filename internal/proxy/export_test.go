package proxy

// WithRoot overrides the filesystem root for the proxy manager.
func WithRoot(path string) func(o *options) {
	return func(o *options) {
		o.root = path
	}
}

const ConfHeader = confHeader
const DefaultEnvConfigPath = defaultEnvConfigPath
const DefaultAPTConfigPath = defaultAPTConfigPath
