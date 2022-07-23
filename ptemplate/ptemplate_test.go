package ptemplate

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestStripHTML(t *testing.T) {
	require.Equal(t, "hello", stripHTML("hello"))
	require.Equal(t, "hello there user", stripHTML(`<a href=""> hello <strong>there</strong> user </p>`))
}
