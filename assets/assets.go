package assets

import _ "embed"

// LogoSVG is the full Seol logo.
//
//go:embed seol-logo.svg
var LogoSVG []byte

// IconSVG is the simplified browser icon.
//
//go:embed seol-icon.svg
var IconSVG []byte
