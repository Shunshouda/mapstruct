package generator

import "testing"

func TestQualifiedContainerIdentityPreservesShape(t *testing.T) {
	g := &Generator{}
	for _, tc := range []struct {
		source, dest string
		same         bool
	}{
		{"[]infra.ShopPosProductSource", "[]ShopPosProductSource", true},
		{"*infra.Source", "*Source", true},
		{"[]*infra.Source", "[]*Source", true},
		{"*infra.Source", "Source", false},
		{"[]infra.Source", "[]*Source", false},
		{"[]infra.Source", "Source", false},
		{"[]infra.Source", "[]Other", false},
	} {
		if got := g.isSameTypeDifferentPackage(tc.source, tc.dest, StructMapping{}); got != tc.same {
			t.Fatalf("%s -> %s identity=%v", tc.source, tc.dest, got)
		}
	}
}
