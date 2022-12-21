package main

import (
	_ "embed"
	"os"
	"text/template"
)

//go:embed gomas.service
var gomasServiceEmbed string

type GomasServiceParams struct {
	BinaryPath string
	User       string
}

func SystemdServiceFile() {
	tmpl := template.New("gomas.service")
	tmpl, err := tmpl.Parse(gomasServiceEmbed)
	if err != nil {
		panic(err)
	}

	path, err := os.Executable()
	if err != nil {
		panic(err)
	}

	params := GomasServiceParams{
		BinaryPath: path,
		User: "pi",
	}

	err = tmpl.Execute(os.Stdout, params)
	if err != nil {
		panic(err)
	}
}
