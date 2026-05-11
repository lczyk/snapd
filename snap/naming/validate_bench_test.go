package naming

import "testing"

func BenchmarkValidateSnap(b *testing.B) {
	b.Run("short", func(b *testing.B) {
		for b.Loop() {
			ValidateSnap("hello")
		}
	})
	b.Run("typical", func(b *testing.B) {
		for b.Loop() {
			ValidateSnap("hello-world")
		}
	})
	b.Run("long", func(b *testing.B) {
		for b.Loop() {
			ValidateSnap("hello-world-extra-long-snap-name-ok")
		}
	})
	b.Run("invalid-dash-start", func(b *testing.B) {
		for b.Loop() {
			ValidateSnap("-bad")
		}
	})
	b.Run("invalid-chars", func(b *testing.B) {
		for b.Loop() {
			ValidateSnap("bad_underscore")
		}
	})
}

func BenchmarkValidateInstance(b *testing.B) {
	b.Run("no-key", func(b *testing.B) {
		for b.Loop() {
			ValidateInstance("hello-world")
		}
	})
	b.Run("with-key", func(b *testing.B) {
		for b.Loop() {
			ValidateInstance("hello-world_abc123")
		}
	})
	b.Run("invalid-key-too-long", func(b *testing.B) {
		for b.Loop() {
			ValidateInstance("hello-world_abc1234567890")
		}
	})
}

func BenchmarkValidateApp(b *testing.B) {
	b.Run("valid", func(b *testing.B) {
		for b.Loop() {
			ValidateApp("my-app")
		}
	})
	b.Run("invalid", func(b *testing.B) {
		for b.Loop() {
			ValidateApp("bad_app")
		}
	})
}

func BenchmarkValidateHook(b *testing.B) {
	b.Run("valid", func(b *testing.B) {
		for b.Loop() {
			ValidateHook("configure")
		}
	})
	b.Run("invalid", func(b *testing.B) {
		for b.Loop() {
			ValidateHook("Bad-Hook")
		}
	})
}

func BenchmarkValidatePlug(b *testing.B) {
	for b.Loop() {
		ValidatePlug("network-bind")
	}
}

func BenchmarkValidateSlot(b *testing.B) {
	for b.Loop() {
		ValidateSlot("network")
	}
}

func BenchmarkValidateInterface(b *testing.B) {
	for b.Loop() {
		ValidateInterface("network-bind")
	}
}

func BenchmarkValidateSnapID(b *testing.B) {
	for b.Loop() {
		ValidateSnapID("abcdef1234567890abcdef1234567890")
	}
}
