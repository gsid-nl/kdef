package generator

import (
	"strings"
	"testing"

	"github.com/gsid-nl/kdef/internal/types"
)

func gpuContainer() types.ContainerConfig {
	return types.ContainerConfig{
		Name:  "llm",
		Image: "llama",
		Resources: &types.ResourcesConfig{
			CPURequest: "2",
			CPULimit:   "8",
			Devices:    map[string]int64{"devic.es/dri": 1},
		},
	}
}

func TestGenerateDeploymentV2_DevicesAndSupplementalGroups(t *testing.T) {
	fsGroup := int64(1000)
	manifests := GenerateDeploymentV2(types.DeploymentConfig{
		Name:            "llm",
		Containers:      []types.ContainerConfig{gpuContainer()},
		SecurityContext: &types.SecurityContextConfig{FSGroup: &fsGroup, SupplementalGroups: []int64{992}},
	}, nil)
	out, err := RenderYAML(manifests)
	if err != nil {
		t.Fatalf("render error: %v", err)
	}
	yaml := string(out)
	for _, want := range []string{"devic.es/dri: \"1\"", "fsGroup: 1000", "supplementalGroups:\n        - 992"} {
		if !strings.Contains(yaml, want) {
			t.Errorf("yaml missing %q:\n%s", want, yaml)
		}
	}
	if strings.Count(yaml, "devic.es/dri") != 1 {
		t.Errorf("a device belongs in the limits only:\n%s", yaml)
	}
}

func TestGenerateDaemonSet_SupplementalGroupsWithoutFSGroup(t *testing.T) {
	manifests := GenerateDaemonSet(types.DaemonSetConfig{
		Name:            "probe",
		Containers:      []types.ContainerConfig{gpuContainer()},
		SecurityContext: &types.SecurityContextConfig{SupplementalGroups: []int64{992}},
	})
	out, err := RenderYAML(manifests)
	if err != nil {
		t.Fatalf("render error: %v", err)
	}
	yaml := string(out)
	if !strings.Contains(yaml, "supplementalGroups:") || strings.Contains(yaml, "fsGroup") {
		t.Errorf("want supplementalGroups without fsGroup:\n%s", yaml)
	}
	if !strings.Contains(yaml, "devic.es/dri: \"1\"") {
		t.Errorf("yaml missing the device:\n%s", yaml)
	}
}
