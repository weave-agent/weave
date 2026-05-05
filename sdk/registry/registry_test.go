package registry

import (
	"bytes"
	"log"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegisterAndGet(t *testing.T) {
	r := New[int]()

	r.Register("foo", 42)
	r.Register("bar", 99)

	val, ok := r.Get("foo")
	require.True(t, ok)
	assert.Equal(t, 42, val)

	val, ok = r.Get("bar")
	require.True(t, ok)
	assert.Equal(t, 99, val)

	_, ok = r.Get("missing")
	assert.False(t, ok)
}

func TestRegisterEmptyNamePanics(t *testing.T) {
	r := New[int]()

	assert.Panics(t, func() {
		r.Register("", 1)
	})
}

func TestDuplicateWarn(t *testing.T) {
	var buf bytes.Buffer

	logger := log.New(&buf, "", 0)

	r := New(WithWarn[int](logger, "item"))

	r.Register("x", 1)
	r.Register("x", 2)

	val, ok := r.Get("x")
	require.True(t, ok)
	assert.Equal(t, 1, val) // first wins
	assert.Contains(t, buf.String(), `warning: item "x" already registered; first registration wins`)
}

func TestDuplicatePanic(t *testing.T) {
	r := New(WithPanic[int]("test"))

	r.Register("x", 1)
	assert.Panics(t, func() {
		r.Register("x", 2)
	})
}

func TestDuplicateNoOption(t *testing.T) {
	r := New[int]()

	r.Register("x", 1)
	r.Register("x", 2) // silent first-wins, no panic, no log

	val, ok := r.Get("x")
	require.True(t, ok)
	assert.Equal(t, 1, val)
}

func TestExists(t *testing.T) {
	r := New[string]()

	assert.False(t, r.Exists("a"))

	r.Register("a", "hello")
	assert.True(t, r.Exists("a"))
	assert.False(t, r.Exists("b"))
}

func TestList(t *testing.T) {
	r := New[int]()

	r.Register("charlie", 3)
	r.Register("alpha", 1)
	r.Register("bravo", 2)

	names := r.List()
	assert.Equal(t, []string{"alpha", "bravo", "charlie"}, names)
}

func TestListEmpty(t *testing.T) {
	r := New[int]()

	names := r.List()
	assert.Empty(t, names)
}

func TestAll(t *testing.T) {
	r := New[int]()

	r.Register("charlie", 3)
	r.Register("alpha", 1)
	r.Register("bravo", 2)

	items := r.All()
	assert.Equal(t, []int{1, 2, 3}, items)
}

func TestReset(t *testing.T) {
	r := New[int]()

	r.Register("a", 1)
	r.Register("b", 2)
	require.Equal(t, []string{"a", "b"}, r.List())

	r.Reset()
	assert.Empty(t, r.List())

	_, ok := r.Get("a")
	assert.False(t, ok)
}

func TestConcurrentAccess(t *testing.T) {
	r := New[int]()

	var wg sync.WaitGroup

	const n = 100

	// concurrent writes
	for i := range n {
		wg.Go(func() {
			r.Register(string(rune('A'+i%26)), i)
		})
	}

	// concurrent reads
	for range n {
		wg.Go(func() {
			r.List()
			r.Get("A")
			r.Exists("A")
		})
	}

	wg.Wait()

	// should not panic and should have entries
	assert.NotEmpty(t, r.List())
}

func TestGenericType(t *testing.T) {
	type widget struct {
		Name string
		Size int
	}

	r := New[widget]()
	r.Register("w1", widget{Name: "alpha", Size: 10})

	w, ok := r.Get("w1")
	require.True(t, ok)
	assert.Equal(t, "alpha", w.Name)
	assert.Equal(t, 10, w.Size)
}

func TestWithWarnMessageFormat(t *testing.T) {
	var buf bytes.Buffer

	logger := log.New(&buf, "prefix: ", 0)

	r := New(WithWarn[string](logger, "extension"))
	r.Register("ext1", "first")
	r.Register("ext1", "second")

	output := buf.String()
	assert.Contains(t, output, "prefix:", "should use provided logger prefix")
	assert.Contains(t, output, `extension "ext1"`, "should include label and name")
}

func TestResetThenRegister(t *testing.T) {
	r := New[int]()
	r.Register("a", 1)
	r.Reset()

	r.Register("a", 2)
	val, ok := r.Get("a")
	require.True(t, ok)
	assert.Equal(t, 2, val)
}
