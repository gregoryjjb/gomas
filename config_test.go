package main_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	gomas "gregoryjjb/gomas"
)

func TestOrderedGPIOConfig(t *testing.T) {
	c := newTestConfig(t, gomas.Flags{}, nil, `data_dir = "/data"
pinout = [2, 3, 4, 17]
tracks = ["Roof 1", "Roof 2", "Roof 1"]`)
	require.Equal(t, []int{2, 3, 4, 17}, c.Pinout())
	require.Equal(t, []string{"Roof 1", "Roof 2", "Roof 1"}, c.Tracks())
	show := gomas.NewShowData(c.Tracks())
	require.Len(t, show.Tracks, 3, "duplicate assignments still represent one show track, plus Beat")
	require.Contains(t, show.Tracks, "Roof 1")
	require.Contains(t, show.Tracks, "Roof 2")
}

func TestEmptyGPIOConfig(t *testing.T) {
	for _, raw := range []string{"", "pinout = []\ntracks = []", "pinout = [2, 3]"} {
		c := newTestConfig(t, gomas.Flags{}, nil, "data_dir = \"/data\"\n"+raw)
		require.Empty(t, c.Tracks())
	}
}

func TestRejectInvalidGPIOConfig(t *testing.T) {
	fs := gomas.NewGomasMemFS()
	require.NoError(t, fs.MkdirAll("/data", 0755))
	for name, config := range map[string]string{
		"old pinout":               "[pinout]\nA = 17",
		"duplicate pin":            "pinout = [17, 17]",
		"negative pin":             "pinout = [-1]",
		"invalid pin":              "pinout = [54]",
		"beat mapping":             "pinout = [17]\ntracks = [\"Beat\"]",
		"empty name":               "pinout = [17]\ntracks = [\"\"]",
		"blank name":               "pinout = [17]\ntracks = [\"  \"]",
		"too many tracks":          "pinout = [17]\ntracks = [\"A\", \"B\"]",
		"too many repeated tracks": "pinout = [17]\ntracks = [\"A\", \"A\"]",
		"tracks without pins":      "tracks = [\"A\"]",
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
