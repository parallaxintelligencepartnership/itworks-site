// Package web embeds the itworks.dev templates and static assets.
package web

import "embed"

//go:embed templates static
var FS embed.FS
