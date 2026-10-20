package profile

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCmToDxa(t *testing.T) {
	require.Equal(t, 1440, CmToDxa(2.54)) // ~1 inch
	require.Equal(t, 567, CmToDxa(1.0))
}

func TestPtToHalfPoints(t *testing.T) {
	require.Equal(t, 22, PtToHalfPoints(11))
	require.Equal(t, 28, PtToHalfPoints(14))
}

func TestPageSizeDxa(t *testing.T) {
	w, h, err := PageSizeDxa("A4")
	require.NoError(t, err)
	require.Equal(t, 11906, w)
	require.Equal(t, 16838, h)
}

func TestLoadEmbeddedProfiles(t *testing.T) {
	l, err := Default()
	require.NoError(t, err)
	for _, name := range []string{"corporate_executive", "gost_7_32_2017", "legal_contract"} {
		p, err := l.Get(name)
		require.NoError(t, err, name)
		require.Equal(t, name, p.Name)
		r, err := Resolve(p)
		require.NoError(t, err)
		require.Greater(t, r.ContentWidthDxa, 0)
	}
	_, err = l.Get("missing_profile")
	require.Error(t, err)
}
