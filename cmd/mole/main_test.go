package main

import (
	"bytes"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kiry163/mole/pkg/config"
)

func TestNewMoleCommand(t *testing.T) {
	cmd := NewMoleCommand()
	require.NotNil(t, cmd)
	assert.Equal(t, "mole", cmd.Use)
	assert.True(t, strings.Contains(cmd.Long, config.FormatVersion()))
	assert.True(t, cmd.CompletionOptions.DisableDefaultCmd)
	assert.Nil(t, cmd.PersistentFlags().Lookup("no-color"))
	assert.Nil(t, cmd.Flags().Lookup("debug"))
	assert.Nil(t, cmd.Flags().Lookup("host"))
	assert.Nil(t, cmd.Flags().Lookup("port"))
	assert.NotNil(t, cmd.RunE)

	names := make([]string, 0, len(cmd.Commands()))
	for _, c := range cmd.Commands() {
		names = append(names, c.Name())
	}
	sort.Strings(names)
	assert.Equal(t, []string{"serve", "service", "version"}, names)
}

func TestRootCommand_NoArgsPrintsHelp(t *testing.T) {
	cmd := NewMoleCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs(nil)
	require.NoError(t, cmd.Execute())
	assert.Contains(t, out.String(), "Usage:")
	assert.Contains(t, out.String(), "serve")
}

func TestRootCommand_RejectsRetiredFlags(t *testing.T) {
	for _, arg := range []string{"--no-color", "--no-browser", "--host", "--port", "--debug"} {
		cmd := NewMoleCommand()
		cmd.SetArgs([]string{arg})
		require.Error(t, cmd.Execute(), "%s should be rejected", arg)
	}
}

func TestRootCommand_UnknownCommandFails(t *testing.T) {
	cmd := NewMoleCommand()
	cmd.SetArgs([]string{"dashboard"})
	require.Error(t, cmd.Execute(), "dashboard should be gone")
}
