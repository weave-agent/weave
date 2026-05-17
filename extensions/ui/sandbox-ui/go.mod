module github.com/weave-agent/weave-sandbox-ui

go 1.26.2

require (
	github.com/stretchr/testify v1.11.1
	github.com/weave-agent/weave v0.0.0
	github.com/weave-agent/weave-sandbox v0.0.0-00010101000000-000000000000
)

require (
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace github.com/weave-agent/weave => ../../..

replace github.com/weave-agent/weave-sandbox => ../../../extensions/sandbox
