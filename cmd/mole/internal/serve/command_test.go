package serve

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestServeCommandFlags(t *testing.T) {
	cmd := NewServeCommand()
	require.NotNil(t, cmd)
	assert.NotNil(t, cmd.Flags().Lookup("debug"))
	assert.Nil(t, cmd.Flags().Lookup("host"))
	assert.Nil(t, cmd.Flags().Lookup("port"))
	assert.Nil(t, cmd.Flags().Lookup("no-browser"))
}

func TestServeCommandPassesDebug(t *testing.T) {
	orig := runServe
	defer func() { runServe = orig }()
	var got Options
	runServe = func(opts Options) error {
		got = opts
		return nil
	}

	cmd := NewServeCommand()
	cmd.SetArgs([]string{"-d"})
	require.NoError(t, cmd.Execute())
	assert.True(t, got.Debug)
}

func TestServeCommandHelp(t *testing.T) {
	cmd := NewServeCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"--help"})
	require.NoError(t, cmd.Execute())
	assert.Contains(t, out.String(), "serve")
}
