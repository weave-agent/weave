module weave/ext/ui/sandboxui

go 1.26.2

require (
	github.com/stretchr/testify v1.11.1
	weave v0.0.0
	weave/extensions/sandbox v0.0.0-00010101000000-000000000000
)

require (
	github.com/BurntSushi/toml v1.6.0 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/nniel-ape/gonfig v1.3.0 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace weave => ../../..

replace weave/extensions/sandbox => ../../../extensions/sandbox
