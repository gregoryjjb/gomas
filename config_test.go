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

func TestDiscoveryRoleConfig(t *testing.T) {
	for _, tc := range []struct {
		name   string
		config string
		role   gomas.Role
		host   string
	}{
		{"default", "", gomas.RoleStandalone, "127.0.0.1"},
		{"standalone", "role = \"standalone\"", gomas.RoleStandalone, "127.0.0.1"},
		{"master", "role = \"master\"", gomas.RoleMaster, "127.0.0.1"},
		{"slave", "role = \"slave\"", gomas.RoleSlave, "127.0.0.1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			config := "data_dir = \"/data\"\n" + tc.config
			c := newTestConfig(t, gomas.Flags{}, nil, config)
			require.Equal(t, tc.role, c.Role())
			require.Equal(t, tc.host, c.Host())
			c = newTestConfig(t, gomas.Flags{}, map[string]string{"HOST": "192.168.1.10"}, config)
			require.Equal(t, "192.168.1.10", c.Host())
		})
	}
	fs := gomas.NewGomasMemFS()
	require.NoError(t, fs.MkdirAll("/data", 0755))
	_, err := gomas.ParseToml("data_dir = \"/data\"\nrole = \"typo\"", fs)
	require.ErrorContains(t, err, "invalid role")
}
