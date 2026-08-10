package commands

import "testing"

func TestRegistryIncludesAllKnownCommandModules(t *testing.T) {
	registry, err := Registry()
	if err != nil {
		t.Fatalf("Registry returned error: %v", err)
	}
	want := []string{
		"api.call",
		"apikey.create",
		"apikey.delete",
		"apikey.list",
		"pre-cache-image-task.create",
		"pre-cache-image-task.get",
		"instance.browser.vnc",
		"instance.code.run",
		"instance.create",
		"instance.debug",
		"instance.delete",
		"instance.exec",
		"instance.file.download",
		"instance.file.upload",
		"instance.get",
		"instance.list",
		"instance.login",
		"instance.mobile.adb",
		"instance.mobile.connect",
		"instance.mobile.disconnect",
		"instance.mobile.list",
		"instance.mobile.tunnel",
		"instance.pause",
		"instance.proxy",
		"instance.resume",
		"instance.update",
		"tool.create",
		"tool.delete",
		"tool.fork",
		"tool.get",
		"tool.list",
		"tool.update",
	}
	for _, id := range want {
		if _, ok := registry.Lookup(id); !ok {
			t.Fatalf("registry missing %s", id)
		}
	}
	if got := len(registry.Modules()); got != len(want) {
		t.Fatalf("module count = %d, want %d", got, len(want))
	}
}

func TestRegistryCloudDescriptorsDeclareOutputEffects(t *testing.T) {
	registry, err := Registry()
	if err != nil {
		t.Fatalf("Registry returned error: %v", err)
	}
	want := map[string][]string{
		"api.call":                    {"call:api"},
		"apikey.create":               {"create:apikey"},
		"apikey.delete":               {"delete:apikey"},
		"apikey.list":                 {"list:apikey"},
		"instance.create":             {"create:instance"},
		"instance.delete":             {"delete:instance"},
		"instance.get":                {"read:instance"},
		"instance.list":               {"list:instance"},
		"instance.pause":              {"pause:instance"},
		"instance.resume":             {"resume:instance"},
		"instance.update":             {"update:instance"},
		"pre-cache-image-task.create": {"create:pre-cache-image-task"},
		"pre-cache-image-task.get":    {"read:pre-cache-image-task"},
		"tool.create":                 {"create:tool"},
		"tool.delete":                 {"delete:tool"},
		"tool.get":                    {"read:tool"},
		"tool.list":                   {"list:tool"},
		"tool.update":                 {"update:tool"},
	}
	for id, effects := range want {
		module, ok := registry.Lookup(id)
		if !ok {
			t.Fatalf("registry missing %s", id)
		}
		if got := module.Descriptor.Spec.Output.Effects; !sameStrings(got, effects) {
			t.Fatalf("%s effects = %v, want %v", id, got, effects)
		}
	}
}

func sameStrings(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}
