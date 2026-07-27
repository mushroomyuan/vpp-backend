package util

import (
	"strings"
	"testing"
)

func TestAssertNotEmpty(t *testing.T) {
	t.Parallel()

	if err := AssertNotEmpty("ok", 1, []int{1}, map[string]int{"a": 1}); err != nil {
		t.Fatal(err)
	}

	cases := []any{
		nil,
		"",
		[]int{},
		map[string]int{},
		(*string)(nil),
		0,
		struct{}{},
	}
	for _, c := range cases {
		if err := AssertNotEmpty(c); err == nil {
			t.Fatalf("want empty for %#v", c)
		}
	}

	s := "x"
	if err := AssertNotEmpty(&s); err != nil {
		t.Fatal(err)
	}
	empty := ""
	if err := AssertNotEmpty(&empty); err == nil {
		t.Fatal("pointer to empty string")
	}
}

func TestMarshalString(t *testing.T) {
	t.Parallel()
	got, err := MarshalString(map[string]int{"a": 1})
	if err != nil || !strings.Contains(got, `"a":1`) {
		t.Fatalf("got %q err=%v", got, err)
	}
	got, err = MarshalString("")
	if err != nil || got != `""` {
		t.Fatalf("empty string: %q %v", got, err)
	}
	_, err = MarshalString(make(chan int))
	if err == nil {
		t.Fatal("want marshal error")
	}
}
