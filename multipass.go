// Copyright 2016 Lars Wiegman. All rights reserved. Use of this source code is
// governed by a BSD-style license that can be found in the LICENSE file.

package multipass

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"errors"
	"html/template"
	"log"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"
	"time"

	jose "gopkg.in/square/go-jose.v1"
)

// Portable errors
var (
	ErrInvalidToken = errors.New("invalid token")
)

// Multipass implements the http.Handler interface which can handle
// authentication and authorization of users and resources using signed JWT.
type Multipass struct {
	Resources []string
	Expires   time.Duration

	basepath string
	siteaddr string
	service  HandleService
	tmpl     *template.Template
	mux      *http.ServeMux
}

// NewMultipass returns a new instance of Multipass with reasonalble defaults:
// 2048 bit RSA key pair, `/multipass` basepath a token expiration time of
// 24 hours.
func NewMultipass(siteaddr string) (*Multipass, error) {
	// Generate and set a private key if none is set
	if k := pemDecodePrivateKey([]byte(os.Getenv(PKENV))); k != nil {
		log.Printf("Use private key from enviroment variable named by key %s\n", PKENV)
	} else {
		pk, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			return nil, err
		}
		buf := new(bytes.Buffer)
		if err := pemEncodePrivateKey(buf, pk); err != nil {
			return nil, err
		}
		os.Setenv(PKENV, buf.String())
	}

	// Load HTML templates
	tmpl, err := loadTemplates()
	if err != nil {
		return nil, err
	}

	m := &Multipass{
		Resources: []string{"/"},
		Expires:   time.Hour * 24,
		basepath:  "/multipass",
		siteaddr:  siteaddr,
		service:   DefaultHandleService,
		tmpl:      tmpl,
	}

	// Create the router
	mux := http.NewServeMux()
	mux.HandleFunc(path.Join(m.basepath, "/"), m.rootHandler)
	mux.HandleFunc(path.Join(m.basepath, "/login"), m.loginHandler)
	mux.HandleFunc(path.Join(m.basepath, "/confirm"), m.confirmHandler)
	mux.HandleFunc(path.Join(m.basepath, "/signout"), m.signoutHandler)
	mux.HandleFunc(path.Join(m.basepath, "/pub.cer"), m.publickeyHandler)
	m.mux = mux

	return m, nil
}

// BasePath return the base path.
func (m *Multipass) BasePath() string {
	return m.basepath
}

// SetBasePath overrides the default base path of `/multipass`.
// The given basepath is made absolute before it is set.
func (m *Multipass) SetBasePath(basepath string) {
	p := path.Clean(basepath)
	if len(p) == 0 {
		return
	}
	if p[len(p)-1] != '/' {
		m.basepath = path.Join("/", p)
		return
	}
	m.basepath = p
}

// SetHandleService overrides the default HandleService.
func (m *Multipass) SetHandleService(s HandleService) {
	m.service = s
}

// ServeHTTP satisfies the ServeHTTP interface
func (m *Multipass) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h, p := m.mux.Handler(r); len(p) > 0 {
		h.ServeHTTP(w, r)
		return
	}
	http.NotFound(w, r)
}

// rootHandler handles the "/" path of the Multipass handler.
// Shows login page when no JWT present
// Show continue or signout page when JWT is valid
// Show token invalid page when JWT is invalid
func (m *Multipass) rootHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {

		// Regular login page
		p := &page{
			Page:        loginPage,
			LoginPath:   path.Join(m.basepath, "login"),
			SignoutPath: path.Join(m.basepath, "signout"),
		}

		// Show login page when there is no token
		tokenStr, err := extractToken(r)
		if err != nil {
			if s := r.URL.Query().Get("url"); !strings.HasPrefix(s, m.basepath) {
				p.NextURL = s
			}
			m.tmpl.ExecuteTemplate(w, "page", p)
			return
		}
		pk := pemDecodePrivateKey([]byte(os.Getenv(PKENV)))
		if pk == nil {
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			return
		}
		var claims *Claims
		if claims, err = validateToken(tokenStr, pk.PublicKey); err != nil {
			p.Page = tokenInvalidPage
			if s := r.URL.Query().Get("url"); !strings.HasPrefix(s, m.basepath) {
				p.NextURL = s
			}
			m.tmpl.ExecuteTemplate(w, "page", p)
			return
		}
		// Authorize handle claim
		if ok := m.service.Listed(claims.Handle); !ok {
			p.Page = tokenInvalidPage
			m.tmpl.ExecuteTemplate(w, "page", p)
			return
		}
		if cookie, err := r.Cookie("next_url"); err == nil {
			p.NextURL = cookie.Value
		}
		p.Page = continueOrSignoutPage
		m.tmpl.ExecuteTemplate(w, "page", p)
		return
	}
	w.WriteHeader(http.StatusMethodNotAllowed)
}

func (m *Multipass) loginHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		if tokenStr := r.URL.Query().Get("token"); len(tokenStr) > 0 {
			cookie := &http.Cookie{
				Name:  "jwt_token",
				Value: tokenStr,
				Path:  "/",
			}
			http.SetCookie(w, cookie)
		}
		if nexturl := r.URL.Query().Get("url"); len(nexturl) > 0 {
			cookie := &http.Cookie{
				Name:  "next_url",
				Value: nexturl,
				Path:  "/",
			}
			http.SetCookie(w, cookie)
		}
		http.Redirect(w, r, m.basepath, http.StatusSeeOther)
		return
	}
	if r.Method == "POST" {
		r.ParseForm()
		handle := r.PostForm.Get("handle")
		if len(handle) > 0 {
			if m.service.Listed(handle) {
				token, err := m.AccessToken(handle)
				if err != nil {
					log.Print(err)
				}
				values := url.Values{}
				if s := r.PostForm.Get("url"); len(s) > 0 {
					values.Set("url", s)
				}
				loginURL, err := NewLoginURL(m.siteaddr, m.basepath, token, values)
				if err != nil {
					log.Print(err)
				}
				if err := m.service.Notify(handle, loginURL.String()); err != nil {
					log.Print(err)
				}
			}
			// Redirect even when the handle is not listed in order to prevent guessing
			location := path.Join(m.basepath, "confirm")
			http.Redirect(w, r, location, http.StatusSeeOther)
			return
		}
		http.Redirect(w, r, m.basepath, http.StatusSeeOther)
		return
	}
	w.WriteHeader(http.StatusMethodNotAllowed)
}

func (m *Multipass) confirmHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		w.Header().Add("Content-Type", "text/html; charset=utf-8")
		p := &page{
			Page: tokenSentPage,
		}
		m.tmpl.ExecuteTemplate(w, "page", p)
		return
	}
	w.WriteHeader(http.StatusMethodNotAllowed)
}

// signoutHandler deletes the jwt_token cookie and redirect to the login
// location.
func (m *Multipass) signoutHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == "POST" {
		if cookie, err := r.Cookie("jwt_token"); err == nil {
			cookie.Expires = time.Now().AddDate(-1, 0, 0)
			cookie.MaxAge = -1
			cookie.Path = "/"
			http.SetCookie(w, cookie)
		}
		http.Redirect(w, r, m.basepath, http.StatusSeeOther)
		return
	}
	w.WriteHeader(http.StatusMethodNotAllowed)
}

// publickeyHandler writes the public key to the given ResponseWriter to allow
// other to validate Multipass signed tokens.
func (m *Multipass) publickeyHandler(w http.ResponseWriter, r *http.Request) {
	pk := pemDecodePrivateKey([]byte(os.Getenv(PKENV)))
	if pk == nil {
		w.WriteHeader(http.StatusInternalServerError)
	}
	err := pemEncodePublicKey(w, &pk.PublicKey)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
	}
	w.Header().Set("Content-Type", "application/pkix-cert")
}

// AccessToken returns a new signed and serialized token with the given handle
// as a claim.
func (m *Multipass) AccessToken(handle string) (tokenStr string, err error) {
	claims := &Claims{
		Handle:    handle,
		Resources: m.Resources,
		Expires:   time.Now().Add(m.Expires).Unix(),
	}
	pk := pemDecodePrivateKey([]byte(os.Getenv(PKENV)))
	if pk == nil {
		return "", err
	}
	signer, err := jose.NewSigner(jose.PS512, pk)
	if err != nil {
		return "", err
	}
	return accessToken(signer, claims)
}

// NewLoginURL returns a login url which can be used as a time limited login.
// Optional values will be encoded in the login URL.
func NewLoginURL(siteaddr, basepath, token string, v url.Values) (*url.URL, error) {
	u, err := url.Parse(siteaddr)
	if err != nil {
		return u, err
	}
	u.Path = path.Join(basepath, "login")
	v.Set("token", token)
	u.RawQuery = v.Encode()
	return u, nil
}

// TokenHandler validates the token in the request before it writes the response.
func TokenHandler(w http.ResponseWriter, r *http.Request, m *Multipass) (int, error) {
	// Extract token from HTTP header, query parameter or cookie
	tokenStr, err := extractToken(r)
	if err != nil {
		return http.StatusUnauthorized, ErrInvalidToken
	}
	var claims *Claims
	pk := pemDecodePrivateKey([]byte(os.Getenv(PKENV)))
	if pk == nil {
		return http.StatusUnauthorized, ErrInvalidToken
	}
	if claims, err = validateToken(tokenStr, pk.PublicKey); err != nil {
		return http.StatusUnauthorized, ErrInvalidToken
	}
	// Authorize handle claim
	if ok := m.service.Listed(claims.Handle); !ok {
		return http.StatusUnauthorized, ErrInvalidToken
	}
	// Verify path claim
	var match bool
	for _, path := range claims.Resources {
		if strings.HasPrefix(r.URL.Path, path) {
			match = true
			continue
		}
	}
	if !match {
		return http.StatusForbidden, errors.New("forbidden")
	}

	// Pass on authorized handle to downstream handlers
	r.Header.Set("Multipass-Handle", claims.Handle)
	return http.StatusOK, nil
}
