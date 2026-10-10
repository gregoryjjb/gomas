package main_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	gomas "gregoryjjb/gomas"
)

func TestNamedGPIOConfig(t *testing.T) {
	c := newTestConfig(t, gomas.Flags{}, nil, `data_dir = "/data"
[pinout]
"Roof 2" = 27
"Roof 1" = 17
"Fence 1" = 22`)
	require.Equal(t, []string{"Fence 1", "Roof 1", "Roof 2"}, c.ChannelNames())
	require.Equal(t, map[string]int{"Fence 1": 22, "Roof 1": 17, "Roof 2": 27}, c.Pinout())
}

func TestRejectInvalidGPIOConfig(t *testing.T) {
	fs := gomas.NewGomasMemFS()
	require.NoError(t, fs.MkdirAll("/data", 0755))
	for name, config := range map[string]string{
		"old pinout":    "pinout = [17]",
		"duplicate pin": "[pinout]\nA = 17\nB = 17",
		"negative pin":  "[pinout]\nA = -1",
		"invalid pin":   "[pinout]\nA = 54",
		"beat mapping":  "[pinout]\nBeat = 17",
		"empty name":    "[pinout]\n\"\" = 17",
	} {
		t.Run(name, func(t *testing.T) {
			_, err := gomas.ParseToml("data_dir = \"/data\"\n"+config, fs)
			require.Error(t, err)
		})
	}
}
