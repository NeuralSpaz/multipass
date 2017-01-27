// Copyright 2017 NeuralSpaz@guthub. All rights reserved.
package multipass

import "testing"

func TestRandomString(t *testing.T) {
	var s string
	for i := 0; i < 1024; i++ {
		s = randomStringGen(i)
		if len(s) != i {
			t.Errorf("want RandomString Length %d, got %d", i, len(s))
		}
	}
}

func BenchmarkRandomString(b *testing.B) {
	var n = 32
	for i := 0; i < b.N; i++ {
		randomStringGen(n)
	}
}
