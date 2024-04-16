package main

import (
	_ "embed"
	"fmt"
	"os"
	"text/template"
)

//go:embed gomas.service
var gomasServiceEmbed string

type GomasServiceParams struct {
	BinaryPath string
	User       string
}

func SystemdServiceFile(flags Flags) error {
	tmpl := template.New("gomas.service")
	tmpl, err := tmpl.Parse(gomasServiceEmbed)
	if err != nil {
		return err
	}

	path, err := os.Executable()
	if err != nil {
		return err
	}

	user := flags.User
	if user == "" {
		return fmt.Errorf("user cannot be blank (set with --user)")
	}

	params := GomasServiceParams{
		BinaryPath: path,
		User:       user,
	}

	err = tmpl.Execute(os.Stdout, params)
	if err != nil {
		return err
	}

	return nil
}
