package snap

import (
	"fmt"
	"testing"
)

func BenchmarkSplitInstanceName(b *testing.B) {
	b.Run("simple", func(b *testing.B) {
		for b.Loop() {
			SplitInstanceName("hello")
		}
	})
	b.Run("with-key", func(b *testing.B) {
		for b.Loop() {
			SplitInstanceName("hello_abc123")
		}
	})
	b.Run("with-key-longer", func(b *testing.B) {
		for b.Loop() {
			SplitInstanceName("hello-world-app_abc123def456")
		}
	})
}

func BenchmarkInstanceName(b *testing.B) {
	b.Run("simple", func(b *testing.B) {
		for b.Loop() {
			InstanceName("hello", "")
		}
	})
	b.Run("with-key", func(b *testing.B) {
		for b.Loop() {
			InstanceName("hello", "abc123")
		}
	})
}

func BenchmarkSplitSnapApp(b *testing.B) {
	b.Run("simple", func(b *testing.B) {
		for b.Loop() {
			SplitSnapApp("hello")
		}
	})
	b.Run("dotted", func(b *testing.B) {
		for b.Loop() {
			SplitSnapApp("hello-world.my-app")
		}
	})
}

func BenchmarkJoinSnapApp(b *testing.B) {
	b.Run("simple", func(b *testing.B) {
		for b.Loop() {
			JoinSnapApp("hello", "hello")
		}
	})
	b.Run("different", func(b *testing.B) {
		for b.Loop() {
			JoinSnapApp("hello-world", "my-app")
		}
	})
	b.Run("with-instance-key", func(b *testing.B) {
		for b.Loop() {
			JoinSnapApp("hello-world_abc123", "my-app")
		}
	})
}

func BenchmarkSortServices(b *testing.B) {
	makeApp := func(name string, after, before []string) *AppInfo {
		return &AppInfo{
			Snap:  &Info{SuggestedName: "test", SideInfo: SideInfo{RealName: "test"}},
			Name:  name,
			After: after,
			Before: before,
		}
	}

	linear := []*AppInfo{
		makeApp("a", nil, []string{"b"}),
		makeApp("b", []string{"a"}, []string{"c"}),
		makeApp("c", []string{"b"}, []string{"d"}),
		makeApp("d", []string{"c"}, nil),
	}

	many := make([]*AppInfo, 50)
	for i := range many {
		many[i] = makeApp(fmt.Sprintf("svc%d", i), nil, nil)
	}
	for i := range many[:len(many)-1] {
		many[i].Before = []string{fmt.Sprintf("svc%d", i+1)}
		many[i+1].After = []string{fmt.Sprintf("svc%d", i)}
	}

	// dependency diamond: a -> {b, c} -> d
	diamond := []*AppInfo{
		makeApp("a", nil, []string{"b", "c"}),
		makeApp("b", []string{"a"}, []string{"d"}),
		makeApp("c", []string{"a"}, []string{"d"}),
		makeApp("d", []string{"b", "c"}, nil),
	}

	b.Run("linear-4", func(b *testing.B) {
		for b.Loop() {
			SortServices(linear)
		}
	})
	b.Run("chain-50", func(b *testing.B) {
		for b.Loop() {
			SortServices(many)
		}
	})
	b.Run("diamond", func(b *testing.B) {
		for b.Loop() {
			SortServices(diamond)
		}
	})
}
