// Package relay embeds the Python relay script used inside containers.
package relay

import _ "embed"

// Script is the Python relay that keeps claude alive across SSH disconnects.
//
//go:embed relay.py
var Script []byte
