// Copyright 2017 NeuralSpaz@guthub. All rights reserved.
package multipass

import (
	"crypto/rand"
	"errors"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Charater set for building url safe randomish strings
const urlSafeChars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ1234567890-_"

// size of urlSafeChars
const urlSafeCharsSize = 64

//generates random(ish) url safe strings of length n
func randomStringGen(n int) string {
	s := rand.Reader
	str := make([]byte, n)
	for i := range str {
		index, _ := rand.Int(s, big.NewInt(64))
		str[i] = urlSafeChars[index.Int64()]
	}
	return string(str)
}

// Holds the state for shortened url to token urls
type lookuptable struct {
	sync.RWMutex
	table map[string]string
}

// adds an entry to lookuptable
func (l *lookuptable) add(key, value string, expire time.Duration) error {
	l.Lock()
	defer l.Unlock()
	// check to make sure key is not in use
	if _, ok := l.table[key]; ok {
		return errors.New("Lookup table key in use")
	}
	l.table[key] = value
	go func() {
		autoExpire := time.NewTimer(expire)
		<-autoExpire.C
		l.delete(key)
	}()
	return nil
}

// deletes an entry to lookuptable
func (l *lookuptable) delete(key string) {
	l.Lock()
	defer l.Unlock()
	delete(l.table, key)
}

// returns value then destorys entry
func (l *lookuptable) lookup(key string) (string, bool) {
	l.Lock()
	defer l.Unlock()
	v, ok := l.table[key]
	delete(l.table, key)
	return v, ok
}

func (m *Multipass) shortHandler(w http.ResponseWriter, r *http.Request) {
	req := strings.TrimPrefix(r.RequestURI, m.Basepath()+"/s/")
	if url, ok := m.shortURL.lookup(req); ok {
		http.Redirect(w, r, url, http.StatusTemporaryRedirect)
		return
	}
	http.Redirect(w, r, m.Basepath(), http.StatusTemporaryRedirect)
	return
}
