package main

import (
	"embed"
	"html/template"
	"io/fs"
	"net/http"
	"sync"

	"github.com/rs/zerolog/log"
)

//go:embed editor
var editorEmbed embed.FS

func GetEditorFS() http.FileSystem {
	log.Debug().Msg("Reading embedded editor files")
	fsys := fs.FS(editorEmbed)
	editorFiles, _ := fs.Sub(fsys, "editor")
	return http.FS(editorFiles)
}

//go:embed templates
var templatesEmbed embed.FS

var (
	templates    *template.Template
	templatesErr error
)

var getTemplates = sync.OnceFunc(func() {
	templateFS, _ := fs.Sub(templatesEmbed, "templates")
	templates, templatesErr = template.ParseFS(templateFS, "*.html")

	if templatesErr != nil {
		log.Err(templatesErr).Msg("Templates failed to parse")
	}
})

func GetTemplates() (*template.Template, error) {
	getTemplates()

	return templates, templatesErr
}

//go:embed static
var staticEmbed embed.FS

func GetStaticFS() (http.FileSystem, error) {
	log.Debug().Msg("Reading embedded static files")
	fsys := fs.FS(staticEmbed)

	staticFiles, err := fs.Sub(fsys, "static")
	if err != nil {
		return nil, err
	}

	return http.FS(staticFiles), nil
}
