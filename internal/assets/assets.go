// Package assets holds the embedded application icon.
package assets

import _ "embed"

// IconPNG is the 256x256 transparent application icon.
//
//go:embed icon.png
var IconPNG []byte
