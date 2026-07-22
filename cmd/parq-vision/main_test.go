package main

import (
	"testing"

	"github.com/blacktop/go-termimg"
	"github.com/stretchr/testify/assert"
)

func TestDetectBestProtocol(t *testing.T) {
	protocol := detectBestProtocol()
	assert.NotEqual(t, termimg.Unsupported, protocol, "detectBestProtocol should not return Unsupported")
}

func TestPrintLogo(t *testing.T) {
	assert.NotPanics(t, func() {
		printLogo()
	})
}
