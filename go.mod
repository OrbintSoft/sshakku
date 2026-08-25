module github.com/OrbintSoft/sshakku

go 1.26

toolchain go1.26.5

require golang.org/x/sys v0.47.0

require github.com/BurntSushi/toml v1.6.0

require github.com/godbus/dbus/v5 v5.2.2

require (
	github.com/ebitengine/purego v0.10.2
	go.uber.org/goleak v1.3.0
	golang.org/x/crypto v0.55.0
)

require go.yaml.in/yaml/v3 v3.0.5 // indirect

require (
	github.com/gofrs/flock v0.13.0
	github.com/stretchr/testify v1.12.1
)
