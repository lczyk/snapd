package strutil

import (
	"fmt"
	"sort"
	"testing"
)

func BenchmarkListContains(b *testing.B) {
	small := []string{"a", "b", "c", "d", "e"}
	medium := make([]string, 100)
	for i := range medium {
		medium[i] = fmt.Sprintf("item-%d", i)
	}
	large := make([]string, 1000)
	for i := range large {
		large[i] = fmt.Sprintf("item-%d", i)
	}

	b.Run("small-hit", func(b *testing.B) {
		for b.Loop() {
			ListContains(small, "c")
		}
	})
	b.Run("small-miss", func(b *testing.B) {
		for b.Loop() {
			ListContains(small, "z")
		}
	})
	b.Run("medium-hit-first", func(b *testing.B) {
		for b.Loop() {
			ListContains(medium, "item-0")
		}
	})
	b.Run("medium-hit-last", func(b *testing.B) {
		for b.Loop() {
			ListContains(medium, "item-99")
		}
	})
	b.Run("medium-miss", func(b *testing.B) {
		for b.Loop() {
			ListContains(medium, "not-there")
		}
	})
	b.Run("large-hit-last", func(b *testing.B) {
		for b.Loop() {
			ListContains(large, "item-999")
		}
	})
	b.Run("large-miss", func(b *testing.B) {
		for b.Loop() {
			ListContains(large, "not-there")
		}
	})
}

func BenchmarkSortedListContains(b *testing.B) {
	small := []string{"a", "b", "c", "d", "e"}
	sort.Strings(small)
	medium := make([]string, 100)
	for i := range medium {
		medium[i] = fmt.Sprintf("item-%d", i)
	}
	sort.Strings(medium)
	large := make([]string, 1000)
	for i := range large {
		large[i] = fmt.Sprintf("item-%d", i)
	}
	sort.Strings(large)

	b.Run("small-hit", func(b *testing.B) {
		for b.Loop() {
			SortedListContains(small, "c")
		}
	})
	b.Run("medium-hit", func(b *testing.B) {
		for b.Loop() {
			SortedListContains(medium, "item-50")
		}
	})
	b.Run("medium-miss", func(b *testing.B) {
		for b.Loop() {
			SortedListContains(medium, "zzz")
		}
	})
	b.Run("large-hit-end", func(b *testing.B) {
		for b.Loop() {
			SortedListContains(large, "item-999")
		}
	})
	b.Run("large-miss", func(b *testing.B) {
		for b.Loop() {
			SortedListContains(large, "zzz")
		}
	})
}

func BenchmarkParseByteSize(b *testing.B) {
	b.Run("bytes", func(b *testing.B) {
		for b.Loop() {
			ParseByteSize("500B")
		}
	})
	b.Run("kilobytes", func(b *testing.B) {
		for b.Loop() {
			ParseByteSize("500kB")
		}
	})
	b.Run("megabytes", func(b *testing.B) {
		for b.Loop() {
			ParseByteSize("500MB")
		}
	})
	b.Run("gigabytes", func(b *testing.B) {
		for b.Loop() {
			ParseByteSize("2GB")
		}
	})
	b.Run("no-unit-error", func(b *testing.B) {
		for b.Loop() {
			ParseByteSize("500")
		}
	})
}

func BenchmarkCommaSeparatedList(b *testing.B) {
	b.Run("single", func(b *testing.B) {
		for b.Loop() {
			CommaSeparatedList("foo")
		}
	})
	b.Run("few", func(b *testing.B) {
		for b.Loop() {
			CommaSeparatedList("foo, bar, baz")
		}
	})
	b.Run("many", func(b *testing.B) {
		for b.Loop() {
			CommaSeparatedList("foo, bar, baz, qux, quux, corge, grault, garply, waldo, fred")
		}
	})
	b.Run("with-whitespace", func(b *testing.B) {
		for b.Loop() {
			CommaSeparatedList(" foo ,, bar , baz ")
		}
	})
}

func BenchmarkSortedListsUniqueMerge(b *testing.B) {
	small1 := []string{"a", "c", "e"}
	small2 := []string{"b", "c", "d"}
	med1 := make([]string, 50)
	med2 := make([]string, 50)
	for i := range med1 {
		med1[i] = fmt.Sprintf("a-item-%03d", i)
		med2[i] = fmt.Sprintf("b-item-%03d", i)
	}

	b.Run("small", func(b *testing.B) {
		for b.Loop() {
			SortedListsUniqueMerge(small1, small2)
		}
	})
	b.Run("medium", func(b *testing.B) {
		for b.Loop() {
			SortedListsUniqueMerge(med1, med2)
		}
	})
}

func BenchmarkDeduplicate(b *testing.B) {
	withDups := make([]string, 100)
	for i := range withDups {
		withDups[i] = fmt.Sprintf("item-%03d", i/2)
	}
	noDups := make([]string, 100)
	for i := range noDups {
		noDups[i] = fmt.Sprintf("item-%03d", i)
	}

	b.Run("with-duplicates", func(b *testing.B) {
		for b.Loop() {
			Deduplicate(withDups)
		}
	})
	b.Run("no-duplicates", func(b *testing.B) {
		for b.Loop() {
			Deduplicate(noDups)
		}
	})
}

func BenchmarkSplitUnit(b *testing.B) {
	b.Run("simple", func(b *testing.B) {
		for b.Loop() {
			SplitUnit("500kB")
		}
	})
	b.Run("large-number", func(b *testing.B) {
		for b.Loop() {
			SplitUnit("123456789GB")
		}
	})
	b.Run("no-unit", func(b *testing.B) {
		for b.Loop() {
			SplitUnit("123456789")
		}
	})
}
