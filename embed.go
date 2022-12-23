package main

import (
	"embed"
	"html/template"
	"io/fs"
	"net/http"

	"github.com/rs/zerolog/log"
)

//go:embed www/index.html
var indexTemplateEmbed string
var indexTemplate *template.Template

func GetIndexTemplate() (*template.Template, error) {
	if NoEmbed {
		// Dynamically read & build
		log.Debug().Msg("Reading index.html template dynamically from filesystem")
		return template.ParseFiles("www/index.html")
	}
	
	if indexTemplate == nil{
		// Build new from embed and cache
		log.Debug().Msg("Caching embedded index.html")
		tmpl, err := template.New("index.html").Parse(indexTemplateEmbed)
		if err != nil {
			return nil, err
		}
		indexTemplate = tmpl
	}

	return indexTemplate, nil
}

//go:embed www/editor
var editorEmbed embed.FS

func GetEditorFS() http.FileSystem {
	if NoEmbed {
		log.Debug().Msg("Reading editor files dynamically from filesystem")
		return http.Dir("www/editor")
	}

	log.Debug().Msg("Reading embedded editor files")
	fsys := fs.FS(editorEmbed)
	editorFiles, _ := fs.Sub(fsys, "www/editor")
	return http.FS(editorFiles)
}
