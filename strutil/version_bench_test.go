package strutil

import "testing"

func BenchmarkVersionCompare(b *testing.B) {
	b.Run("equal", func(b *testing.B) {
		for b.Loop() {
			VersionCompare("1.0", "1.0")
		}
	})
	b.Run("greater", func(b *testing.B) {
		for b.Loop() {
			VersionCompare("2.0", "1.0")
		}
	})
	b.Run("less", func(b *testing.B) {
		for b.Loop() {
			VersionCompare("1.0", "2.0")
		}
	})
	b.Run("long-equal", func(b *testing.B) {
		for b.Loop() {
			VersionCompare("2.54.3+git1.abcdef12-1", "2.54.3+git1.abcdef12-1")
		}
	})
	b.Run("long-subversion", func(b *testing.B) {
		for b.Loop() {
			VersionCompare("2.54.3+git1.abcdef12-1", "2.54.3+git1.abcdef12-2")
		}
	})
	b.Run("differing-segments", func(b *testing.B) {
		for b.Loop() {
			VersionCompare("2.54.3", "2.54.3.1")
		}
	})
	b.Run("invalid", func(b *testing.B) {
		for b.Loop() {
			VersionCompare("1:", "1.0")
		}
	})
}
