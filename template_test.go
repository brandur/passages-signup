package main

import (
	"testing"

	assert "github.com/stretchr/testify/require"
)

func TestStripHTML(t *testing.T) {
	assert.Equal(t, "hello", stripHTML("hello"))
	assert.Equal(t, "hello there user", stripHTML(`<a href=""> hello <strong>there</strong> user </p>`))
}
