package naturalness

import (
	"math"
	"testing"
)

func TestAnalyze_EmptyFile(t *testing.T) {
	src := `package foo_test`
	r, err := AnalyzeSource(src, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.TestCount != 0 {
		t.Fatalf("expected 0 tests, got %d", r.TestCount)
	}
}

func TestAnalyze_StdlibAssertions(t *testing.T) {
	src := `package foo_test
import "testing"
func TestFoo(t *testing.T) {
	if x := 1; x != 1 {
		t.Errorf("bad: %d", x)
	}
	if y := 2; y != 2 {
		t.Fatalf("bad2: %d", y)
	}
}`
	r, err := AnalyzeSource(src, []string{"Foo"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.TestCount != 1 {
		t.Fatalf("TestCount=%d, want 1", r.TestCount)
	}
	if r.AssertionRatio != 2.0 {
		t.Fatalf("AssertionRatio=%v, want 2.0", r.AssertionRatio)
	}
	if r.NoAssertionsPct != 0 {
		t.Fatalf("NoAssertionsPct=%v, want 0", r.NoAssertionsPct)
	}
	// TestFoo tokenised to ["Foo"] which matches focal "Foo" perfectly.
	if r.TestNameScore < 99 {
		t.Fatalf("TestNameScore=%v, want ~100", r.TestNameScore)
	}
}

func TestAnalyze_NoAssertions(t *testing.T) {
	src := `package foo_test
import "testing"
func TestFoo(t *testing.T) {
	_ = 42
}`
	r, _ := AnalyzeSource(src, nil)
	if r.NoAssertionsPct != 100 {
		t.Fatalf("NoAssertionsPct=%v, want 100", r.NoAssertionsPct)
	}
	if r.AssertionRatio != 0 {
		t.Fatalf("AssertionRatio=%v, want 0", r.AssertionRatio)
	}
}

func TestAnalyze_Testify(t *testing.T) {
	src := `package foo_test
import (
	"testing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)
func TestUserCreate(t *testing.T) {
	u := NewUser("a")
	assert.Equal(t, "a", u.Name)
	require.NoError(t, nil)
	assert.True(t, u.Active)
}`
	r, _ := AnalyzeSource(src, []string{"UserCreate"})
	if r.TestCount != 1 {
		t.Fatalf("TestCount=%d", r.TestCount)
	}
	if r.AssertionRatio != 3 {
		t.Fatalf("AssertionRatio=%v, want 3", r.AssertionRatio)
	}
	if r.ErrorAssertionsPct != 100 {
		t.Fatalf("ErrorAssertionsPct=%v, want 100", r.ErrorAssertionsPct)
	}
	// VarNameScore: u = NewUser(...) → similarity("u","user") high but < 100
	if r.VarNameScore == 0 {
		t.Fatal("VarNameScore should be > 0")
	}
}

func TestAnalyze_DuplicateAssertions(t *testing.T) {
	src := `package foo_test
import (
	"testing"
	"github.com/stretchr/testify/assert"
)
func TestDup(t *testing.T) {
	assert.Equal(t, 1, x)
	assert.Equal(t, 1, x)
}`
	r, _ := AnalyzeSource(src, nil)
	if r.DuplicateAssertionsPct != 100 {
		t.Fatalf("DuplicateAssertionsPct=%v, want 100", r.DuplicateAssertionsPct)
	}
}

func TestAnalyze_NilOnly(t *testing.T) {
	src := `package foo_test
import (
	"testing"
	"github.com/stretchr/testify/assert"
)
func TestNilOnly(t *testing.T) {
	assert.Nil(t, x)
	assert.NotNil(t, y)
}`
	r, _ := AnalyzeSource(src, nil)
	if r.NilOnlyAssertionsPct != 100 {
		t.Fatalf("NilOnlyAssertionsPct=%v, want 100", r.NilOnlyAssertionsPct)
	}
	if r.DuplicateAssertionsPct != 0 {
		t.Fatalf("DuplicateAssertionsPct=%v, want 0", r.DuplicateAssertionsPct)
	}
}

func TestAnalyze_ErrorAssertions(t *testing.T) {
	src := `package foo_test
import (
	"testing"
	"github.com/stretchr/testify/require"
)
func TestErr(t *testing.T) {
	err := doWork()
	require.Error(t, err)
}`
	r, _ := AnalyzeSource(src, nil)
	if r.ErrorAssertionsPct != 100 {
		t.Fatalf("ErrorAssertionsPct=%v", r.ErrorAssertionsPct)
	}
}

func TestAnalyze_MixedTestsNotSingular(t *testing.T) {
	src := `package foo_test
import (
	"testing"
	"github.com/stretchr/testify/assert"
)
func TestA(t *testing.T) {
	assert.Equal(t, 1, 1)
}
func TestB(t *testing.T) {
	// no assertions
	_ = 42
}
func TestC(t *testing.T) {
	assert.Nil(t, x)
	assert.Nil(t, y)
}`
	r, _ := AnalyzeSource(src, []string{"A"})
	if r.TestCount != 3 {
		t.Fatalf("TestCount=%d", r.TestCount)
	}
	if math.Abs(r.NoAssertionsPct-33.333) > 1 {
		t.Fatalf("NoAssertionsPct=%v, want ~33.3", r.NoAssertionsPct)
	}
	// TestC has 2 Nil asserts → nil-only + duplicate.
	if math.Abs(r.NilOnlyAssertionsPct-33.333) > 1 {
		t.Fatalf("NilOnlyAssertionsPct=%v", r.NilOnlyAssertionsPct)
	}
}

func TestAnalyze_IgnoresTestMainAndHelpers(t *testing.T) {
	src := `package foo_test
import "testing"
func TestMain(m *testing.M) {}
func helper(t *testing.T) {}
func TestReal(t *testing.T) { t.Errorf("fail") }`
	r, _ := AnalyzeSource(src, nil)
	if r.TestCount != 1 {
		t.Fatalf("TestCount=%d, want 1", r.TestCount)
	}
}

func TestSimilarity(t *testing.T) {
	cases := []struct {
		a, b string
		want float64
	}{
		{"user", "user", 1.0},
		{"user", "users", 0.8},
		{"", "abc", 0.0},
		{"abc", "", 0.0},
	}
	for _, c := range cases {
		if got := similarity(c.a, c.b); math.Abs(got-c.want) > 0.01 {
			t.Errorf("similarity(%q,%q)=%v, want %v", c.a, c.b, got, c.want)
		}
	}
}

func TestTokenizeTestName(t *testing.T) {
	cases := []struct {
		name string
		want []string
	}{
		{"TestFoo", []string{"Foo"}},
		{"TestFooBar", []string{"Foo", "Bar"}},
		{"TestFoo_Bar", []string{"Foo", "Bar"}},
		{"TestFooBar_BazQux", []string{"Foo", "Bar", "Baz", "Qux"}},
	}
	for _, c := range cases {
		got := tokenizeTestName(c.name)
		if len(got) != len(c.want) {
			t.Errorf("tokenizeTestName(%q)=%v, want %v", c.name, got, c.want)
			continue
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("tokenizeTestName(%q)=%v, want %v", c.name, got, c.want)
				break
			}
		}
	}
}
