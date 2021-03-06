// Copyright 2016 Lars Wiegman. All rights reserved. Use of this source code is
// governed by a BSD-style license that can be found in the LICENSE file.

package multipass

import (
	"html/template"
	"os"
	"path"
	"time"

	"github.com/NeuralSpaz/multipass/services/io"
)

// Option describes a functional option for configuring the Multipass
// instance.
type Option func(*Multipass)

// Basepath overrides the default base path of `/multipass`.
// The given basepath is made absolute before it is set.
func Basepath(basepath string) Option {
	return func(m *Multipass) {
		p := path.Join("/", path.Clean(basepath))
		if p == "/" {
			return
		}
		m.opts.Basepath = p
	}
}

// Expires sets the maximum age for JWT tokens. When a token exceeds the maximum
// age it is no longer valid.
func Expires(d time.Duration) Option {
	return func(m *Multipass) {
		m.opts.Expires = d
	}
}

// CSRF sets a bool whether to protect against CSRF attaks. The default is true.
func CSRF(b bool) Option {
	return func(m *Multipass) {
		m.opts.CSRF = b
	}
}

// Template sets the template to use for generating the web interface.
func Template(t template.Template) Option {
	return func(m *Multipass) {
		m.opts.Template = t
	}
}

// Service sets the UserService and overrides DefaultUserService.
func Service(s UserService) Option {
	return func(m *Multipass) {
		m.opts.Service = s
	}
}

// ShortURL set whether to use shorten urls
func Short(b bool) Option {
	return func(m *Multipass) {
		m.opts.Short = b
	}
}

// ShortURL set whether to use shorten urls
func ShortExpires(t time.Duration) Option {
	return func(m *Multipass) {
		m.opts.ShortExpires = t
	}
}

// ShortLength set the length of the login url suffix
func ShortLength(i int) Option {
	return func(m *Multipass) {
		m.opts.ShortLength = i
	}
}

// parseOptions parses the given option functions and returns a configured
// Multipass instance.
func parseOptions(opts ...Option) *Multipass {
	m := new(Multipass)

	// set default for the options
	m.opts.Expires = time.Hour * 24
	m.opts.Basepath = "/multipass"
	m.opts.Short = true
	m.opts.ShortExpires = time.Minute * 1
	m.opts.ShortLength = 80
	m.opts.Service = io.NewUserService(os.Stdout)
	m.opts.Template = *loadTemplates()
	m.opts.CSRF = true

	// range over each option function and apply it to the instance.
	for _, option := range opts {
		option(m)
	}

	return m
}
