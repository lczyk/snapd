package snap

import (
	"testing"

	"gopkg.in/yaml.v2"
)

func BenchmarkValidate(b *testing.B) {
	old := SanitizePlugsSlots
	SanitizePlugsSlots = func(snapInfo *Info) {}
	defer func() { SanitizePlugsSlots = old }()

	typicalInfo, err := InfoFromSnapYaml([]byte(snapYamlTypical))
	if err != nil {
		b.Fatal(err)
	}
	complexInfo, err := InfoFromSnapYaml([]byte(snapYamlComplex))
	if err != nil {
		b.Fatal(err)
	}

	b.Run("typical", func(b *testing.B) {
		for b.Loop() {
			Validate(typicalInfo)
		}
	})
	b.Run("complex", func(b *testing.B) {
		for b.Loop() {
			Validate(complexInfo)
		}
	})
}

func BenchmarkValidateVersion(b *testing.B) {
	b.Run("simple-valid", func(b *testing.B) {
		for b.Loop() {
			ValidateVersion("1.0")
		}
	})
	b.Run("long-valid", func(b *testing.B) {
		for b.Loop() {
			ValidateVersion("3.14.159-26535-g1234567")
		}
	})
	b.Run("empty", func(b *testing.B) {
		for b.Loop() {
			ValidateVersion("")
		}
	})
	b.Run("too-long", func(b *testing.B) {
		for b.Loop() {
			ValidateVersion("this-string-is-way-too-long-for-a-version")
		}
	})
	b.Run("non-graphic", func(b *testing.B) {
		for b.Loop() {
			ValidateVersion("hello\x00world")
		}
	})
}

func BenchmarkValidateApp(b *testing.B) {
	app := &AppInfo{
		Snap: &Info{
			SuggestedName: "test",
			SideInfo:      SideInfo{RealName: "test"},
			Confinement:   StrictConfinement,
			Plugs:         map[string]*PlugInfo{"network-bind": {Interface: "network-bind"}},
		},
		Name:    "my-app",
		Command: "bin/my-app",
		Daemon:  "simple",
		DaemonScope: SystemDaemon,
		StopTimeout: 10,
		StopMode: "sigterm",
		Plugs: make(map[string]*PlugInfo),
	}
	app.Plugs["network-bind"] = &PlugInfo{Interface: "network-bind"}

	b.ResetTimer()
	for b.Loop() {
		ValidateApp(app)
	}
}

func BenchmarkValidateHook(b *testing.B) {
	hook := &HookInfo{
		Snap: &Info{SuggestedName: "test", SideInfo: SideInfo{RealName: "test"}},
		Name: "configure",
	}
	b.ResetTimer()
	for b.Loop() {
		ValidateHook(hook)
	}
}

func BenchmarkValidateAll(b *testing.B) {
	old := SanitizePlugsSlots
	SanitizePlugsSlots = func(snapInfo *Info) {}
	defer func() { SanitizePlugsSlots = old }()

	// Full parse + validate pipeline, as done in ReadInfoFromSnapFile
	yamlData := []byte(snapYamlComplex)
	b.ResetTimer()
	for b.Loop() {
		var y snapYaml
		if err := yaml.Unmarshal(yamlData, &y); err != nil {
			b.Fatal(err)
		}
		info, err := infoFromSnapYaml(yamlData, new(scopedTracker))
		if err != nil {
			b.Fatal(err)
		}
		if err := Validate(info); err != nil {
			b.Fatal(err)
		}
	}
}
