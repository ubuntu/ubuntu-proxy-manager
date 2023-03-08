# ubuntu-proxy-manager

Ubuntu Proxy Manager is a D-Bus mediated service that allows for managing system proxy settings via multiple backends (APT, environment variables and GSettings).

[![Code quality](https://github.com/ubuntu/ubuntu-proxy-manager/workflows/QA/badge.svg)](https://github.com/ubuntu/ubuntu-proxy-manager/actions?query=workflow%3AQA)
[![Code coverage](https://codecov.io/gh/ubuntu/ubuntu-proxy-manager/branch/main/graph/badge.svg)](https://codecov.io/gh/ubuntu/ubuntu-proxy-manager)
[![Go Reference](https://pkg.go.dev/badge/github.com/ubuntu/ubuntu-proxy-manager.svg)](https://pkg.go.dev/github.com/ubuntu/ubuntu-proxy-manager)
[![Go Report Card](https://goreportcard.com/badge/ubuntu/ubuntu-proxy-manager)](https://goreportcard.com/report/ubuntu/ubuntu-proxy-manager)
[![License](https://img.shields.io/badge/License-GPL3.0-blue.svg)](https://github.com/ubuntu/ubuntu-proxy-manager/blob/main/LICENSE)

## Installation

Use APT to install the package on Ubuntu, which will set up all the required configuration files:

```sh
sudo apt install ubuntu-proxy-manager
```

## Usage

The service currently exposes a single D-Bus method, `com.ubuntu.ProxyManager.Apply`, taking 6 string arguments:
- `http` - HTTP proxy
- `https` - HTTPS proxy
- `ftp` - FTP proxy
- `socks` - SOCKS proxy
- `no_proxy` - hosts excluded from proxy
- `auto` - proxy autoconfiguration URL

When calling the function, all 6 arguments must be passsed. Arguments can be skipped by replacing them with empty strings. Keep in mind that this function is not additive and it replaces previously set proxy settings on each call.

``` sh
# Only apply HTTP proxy
gdbus call --system --dest com.ubuntu.ProxyManager \
                    --object-path /com/ubuntu/ProxyManager \
                    --method com.ubuntu.ProxyManager.Apply \
                    "http://example.com:8080" "" "" "" "" ""

# Only set proxy autoconfiguration URL, HTTP proxy is removed
gdbus call --system --dest com.ubuntu.ProxyManager \
                    --object-path /com/ubuntu/ProxyManager \
                    --method com.ubuntu.ProxyManager.Apply \
                    "" "" "" "" "" "http://example.com:8080/proxy.pac"

# Set all proxy settings
gdbus call --system --dest com.ubuntu.ProxyManager \
                    --object-path /com/ubuntu/ProxyManager \
                    --method com.ubuntu.ProxyManager.Apply \
                    "http://example.com:8080" \
                    "https://example.com:8080" \
                    "ftp://example.com:8080" \
                    "socks://example.com:8080" \
                    "localhost,127.0.0.1,::1" \
                    "http://example.com:8080/proxy.pac"

# Remove all previously set proxy settings
gdbus call --system --dest com.ubuntu.ProxyManager \
                    --object-path /com/ubuntu/ProxyManager \
                    --method com.ubuntu.ProxyManager.Apply \
                    "" "" "" "" "" ""
```

Due to the privileged nature of the service, polkit authorization is set in place to only allow admins to execute the `Apply` method.

Some backends do not support all configuration options. These are described below and will be silently skipped on proxy application.

### Proxy URL format

For the individual proxy protocols, the URL must be in the form of:

```
protocol://username:password@host:port
```

It is not mandatory to escape special characters in the username or password. The service will escape any unescaped special character before applying the proxy settings, and will take care not to double-escape already escaped characters.

### `no_proxy` format

The host exclusion setting must be in the form of:

```
localhost,127.0.0.1,::1
```

Hosts can be individually wrapped in single (`'`) or double quotes (`"`), or separated by spaces.

## Supported backends

### Environment variables

Proxy configuration via environment variables set in `/etc/environment.d/99ubuntu-proxy-manager.conf`.

Unsupported settings: `auto`

### APT

APT proxy configuration set in `/etc/apt/apt.conf.d/99ubuntu-proxy-manager`.

Unsupported settings: `no_proxy`, `auto`

### GSettings

GSettings proxy configuration set in `/usr/share/glib-2.0/schemas/99_ubuntu-proxy-manager.gschema.override`.

This backend is optional and is only active if `glib-compile-schemas` is available in the system `PATH`.

The service will execute `glib-compile-schemas` after applying the settings in order to make the changes visible to GSettings. If an error occurs during the execution of `glib-compile-schemas`, the previous proxy configuration file is restored if applicable.

Autoconfiguration URLs are always prioritzed over manual proxy settings, meaning that if all proxy options are set, the service will set `mode` to `auto` for GSettings to ensure the autoconfiguration URL is used.

## Troubleshooting

The default behavior of the proxy service is to apply the given settings to all backends. If an error occurs in a specific backend, the other backends are not affected and the proxy settings will still be applied to them.

To increase verbosity of the service, append `-d` to the `ExecStart` line of the `ubuntu-proxy-manager` systemd unit file, and run `systemctl daemon-reload`:

```
# cat /lib/systemd/system/ubuntu-proxy-manager.service
...
[Service]
Type=dbus
BusName=com.ubuntu.ProxyManager
ExecStart=/usr/libexec/ubuntu-proxy-manager -d
```

## Development

Your help would be very much appreciated! Check out the [CONTRIBUTING](./CONTRIBUTING.md) document for more information on how to set up the project locally, and how you could collaborate.

## Contact

You are welcome to create a new issue on this repository if you find bugs or wish to make any feature requests.
