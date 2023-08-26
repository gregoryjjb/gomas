package main

import (
	"embed"
	"html/template"
	"io/fs"
	"net/http"
	"os"

	"github.com/rs/zerolog/log"
)

//go:embed editor
var editorEmbed embed.FS

func GetEditorFS() http.FileSystem {
	if NoEmbed {
		log.Debug().Msg("Reading editor files dynamically from filesystem")
		return http.Dir("editor")
	}

	log.Debug().Msg("Reading embedded editor files")
	fsys := fs.FS(editorEmbed)
	editorFiles, _ := fs.Sub(fsys, "editor")
	return http.FS(editorFiles)
}

//go:embed templates
var templatesEmbed embed.FS
var templates *template.Template

func GetTemplates() (*template.Template, error) {
	// Re-parse on every request, for development
	if NoEmbed {
		log.Debug().Msg("Parsing templates from filesystem")
		templateFS := os.DirFS("templates")
		return template.ParseFS(templateFS, "*.html")
	}

	// Use cached parsed
	if templates != nil {
		return templates, nil
	}

	log.Debug().Msg("Parsing templates from embed")

	templateFS, _ := fs.Sub(templatesEmbed, "templates")
	t, err := template.ParseFS(templateFS, "*.html")
	if err != nil {
		return nil, err
	}
	templates = t
	return t, nil
}

// go:embed static
var staticEmbed embed.FS

func GetStatic() (fs.FS, error) {
	if NoEmbed {
		return os.DirFS("static"), nil
	}

	return fs.Sub(staticEmbed, "static")
}
