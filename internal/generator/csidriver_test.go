package generator

import (
	"strings"
	"testing"

	"github.com/gsid-nl/kdef/internal/types"
)

func TestGenerateCSIDriverAndVolume(t *testing.T) {
	y, err := RenderYAML([]Manifest{{Object: GenerateCSIDriver(types.CSIDriverConfig{
		Name: "storage.example.com", LifecycleModes: []string{"Ephemeral"}, FSGroupPolicy: "None",
	})}})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"kind: CSIDriver", "name: storage.example.com", "attachRequired: false", "- Ephemeral", "fsGroupPolicy: None"} {
		if !strings.Contains(string(y), want) {
			t.Errorf("csidriver yaml missing %q:\n%s", want, y)
		}
	}

	v := types.VolumeConfig{Name: "data", MountPath: "/data/cms", CSIDriver: "storage.example.com",
		CSIAttributes: map[string]string{"bucket": "shop"}, CSISecret: "files-shop", ReadOnly: true, MountPropagation: "HostToContainer"}
	vol, mnt := buildVolume(v), buildVolumeMount(v)
	if vol.CSI == nil || vol.CSI.Driver != "storage.example.com" || vol.CSI.VolumeAttributes["bucket"] != "shop" ||
		vol.CSI.NodePublishSecretRef.Name != "files-shop" || !*vol.CSI.ReadOnly {
		t.Errorf("csi volume: %+v", vol.CSI)
	}
	if mnt.MountPropagation == nil || *mnt.MountPropagation != "HostToContainer" {
		t.Errorf("mount propagation: %+v", mnt.MountPropagation)
	}
}
