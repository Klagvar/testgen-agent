package coverage

import (
	"reflect"
	"testing"
)

func TestFilterExecutableLines_Basic(t *testing.T) {
	src := `package main

import (
	"fmt"
	"os"
)

// Add adds two numbers.
func Add(a, b int) int {
	return a + b
}
`
	// Lines: 1=package, 2=blank, 3=import(, 4="fmt", 5="os", 6=),
	// 7=blank, 8=comment, 9=func sig, 10=return, 11=}
	all := []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11}
	got := FilterExecutableLinesFromSource(src, all)
	// Only lines 9 (func sig) and 10 (return) are executable
	want := []int{9, 10}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestFilterExecutableLines_BlockComment(t *testing.T) {
	src := `package main

/*
Multi-line
comment
*/
func Do() {
	x := 1
	_ = x
}
`
	all := []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	got := FilterExecutableLinesFromSource(src, all)
	want := []int{7, 8, 9}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestFilterExecutableLines_SingleImport(t *testing.T) {
	src := `package main

import "fmt"

func main() {
	fmt.Println("hi")
}
`
	all := []int{1, 2, 3, 4, 5, 6, 7}
	got := FilterExecutableLinesFromSource(src, all)
	want := []int{5, 6}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestFilterExecutableLines_Empty(t *testing.T) {
	got := FilterExecutableLinesFromSource("package main\n", nil)
	if len(got) != 0 {
		t.Errorf("expected empty, got %v", got)
	}
}

func TestFilterExecutableLines_OnlyBraces(t *testing.T) {
	src := `package main

func main() {
	if true {
		doSomething()
	}
}
`
	all := []int{3, 4, 5, 6, 7}
	got := FilterExecutableLinesFromSource(src, all)
	// line 3 = func main() { → executable (has code)
	// line 4 = if true { → executable
	// line 5 = doSomething() → executable
	// line 6 = } → just brace
	// line 7 = } → just brace
	want := []int{3, 4, 5}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestFilterExecutableLines_SubsetOfLines(t *testing.T) {
	src := `package main

import "fmt"

func main() {
	fmt.Println("hello")
}
`
	subset := []int{1, 6}
	got := FilterExecutableLinesFromSource(src, subset)
	want := []int{6}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestFilterExecutableLines_InlineBlockComment(t *testing.T) {
	src := `package main

/* single line comment */
func Do() int {
	return 1
}
`
	all := []int{1, 2, 3, 4, 5, 6}
	got := FilterExecutableLinesFromSource(src, all)
	want := []int{4, 5}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestFilterExecutableLines_MixedComments(t *testing.T) {
	src := `package main

// line comment
func A() {
	// inside comment
	x := 1
	_ = x
}
`
	all := []int{1, 2, 3, 4, 5, 6, 7, 8}
	got := FilterExecutableLinesFromSource(src, all)
	want := []int{4, 6, 7}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}
