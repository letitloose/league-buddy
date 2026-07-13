package ui

import (
	"embed"
)

//embeds the html and static directories into the ui.Files filesystem at compile time.

//go:embed "html" "static"
var Files embed.FS
