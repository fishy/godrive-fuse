package gdrive

import (
	"reflect"
	"testing"
)

func TestSplitPath(t *testing.T) {
	for _, c := range []struct {
		Name     string
		Expected []string
	}{
		{
			Name:     "",
			Expected: nil,
		},
		{
			Name:     "//",
			Expected: nil,
		},
		{
			Name:     "foo/../",
			Expected: nil,
		},
		{
			Name:     "/foo",
			Expected: []string{"foo"},
		},
		{
			Name:     "foo/../bar",
			Expected: []string{"bar"},
		},
		{
			Name:     "foo//bar",
			Expected: []string{"foo", "bar"},
		},
	} {
		t.Run(
			c.Name,
			func(t *testing.T) {
				parts := splitPath(c.Name)
				if !reflect.DeepEqual(c.Expected, parts) {
					t.Errorf(
						"splitName(%q) expected %+v, got %+v",
						c.Name,
						c.Expected,
						parts,
					)
				}
			},
		)
	}
}
