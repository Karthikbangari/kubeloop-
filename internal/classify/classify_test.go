package classify

import (
	"testing"

	rs "github.com/kubeloop/kubeloop/internal/rightsizing"
)

func TestClassify(t *testing.T) {
	const Mi = 1 << 20
	cases := []struct {
		name              string
		current, proposed rs.Resources
		want              Class
	}{
		{"cpu reduces", rs.Resources{CPU: 2000, Mem: 512 * Mi}, rs.Resources{CPU: 576, Mem: 512 * Mi}, Waste},
		{"both reduce", rs.Resources{CPU: 2000, Mem: 1024 * Mi}, rs.Resources{CPU: 576, Mem: 428 * Mi}, Waste},
		{"cpu reduces, mem increases -> still waste", rs.Resources{CPU: 2000, Mem: 100 * Mi}, rs.Resources{CPU: 576, Mem: 200 * Mi}, Waste},
		{"both increase -> under-provisioned", rs.Resources{CPU: 500, Mem: 256 * Mi}, rs.Resources{CPU: 2880, Mem: 640 * Mi}, UnderProvisioned},
		{"cpu increases only -> under-provisioned", rs.Resources{CPU: 500, Mem: 512 * Mi}, rs.Resources{CPU: 2880, Mem: 512 * Mi}, UnderProvisioned},
		{"mem increases only -> under-provisioned", rs.Resources{CPU: 500, Mem: 512 * Mi}, rs.Resources{CPU: 500, Mem: 640 * Mi}, UnderProvisioned},
		{"equal -> right-sized", rs.Resources{CPU: 500, Mem: 512 * Mi}, rs.Resources{CPU: 500, Mem: 512 * Mi}, RightSized},
	}
	for _, c := range cases {
		if got := Classify(c.current, c.proposed); got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}

func TestClass_String(t *testing.T) {
	if Waste.String() != "waste" || UnderProvisioned.String() != "under-provisioned" || RightSized.String() != "right-sized" {
		t.Error("Class.String mismatch")
	}
}
