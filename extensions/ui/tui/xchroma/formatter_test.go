package xchroma

import (
	"bytes"
	"testing"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
	chromastyles "github.com/alecthomas/chroma/v2/styles"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewFormatter_GoCode(t *testing.T) {
	f := NewFormatter()
	require.NotNil(t, f)

	style := chromastyles.Fallback
	lexer := lexers.Get("go")
	require.NotNil(t, lexer)

	it, err := lexer.Tokenise(nil, `fmt.Println("hello")`)
	require.NoError(t, err)

	var buf bytes.Buffer

	err = f.Format(&buf, style, it)
	require.NoError(t, err)

	out := buf.String()
	assert.Contains(t, out, "fmt")
	assert.Contains(t, out, "Println")
	assert.Contains(t, out, "hello")
}

func TestNewFormatter_EmptyInput(t *testing.T) {
	f := NewFormatter()
	style := chromastyles.Fallback

	it := func() chroma.Token { return chroma.EOF }

	var buf bytes.Buffer

	err := f.Format(&buf, style, it)
	require.NoError(t, err)
	assert.Empty(t, buf.String())
}

func TestNewFormatter_PlainText(t *testing.T) {
	f := NewFormatter()
	style := chromastyles.Fallback

	tokens := []chroma.Token{
		{Type: chroma.Text, Value: "hello world"},
	}
	it := tokenIterator(tokens)

	var buf bytes.Buffer

	err := f.Format(&buf, style, it)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "hello world")
}

func TestNewFormatter_BoldToken(t *testing.T) {
	f := NewFormatter()

	builder := chromastyles.Fallback.Builder()
	builder.Add(chroma.Keyword, "bold #ff0000")
	style, err := builder.Build()
	require.NoError(t, err)

	tokens := []chroma.Token{
		{Type: chroma.Keyword, Value: "func"},
		{Type: chroma.Text, Value: " main"},
	}
	it := tokenIterator(tokens)

	var buf bytes.Buffer

	err = f.Format(&buf, style, it)
	require.NoError(t, err)

	out := buf.String()
	assert.Contains(t, out, "func")
	assert.Contains(t, out, "main")
	assert.Greater(t, len(out), len("func main"))
}

func TestNewFormatter_MultipleTokenTypes(t *testing.T) {
	f := NewFormatter()

	builder := chromastyles.Fallback.Builder()
	builder.Add(chroma.Keyword, "bold #ff0000")
	builder.Add(chroma.String, "#00ff00")
	builder.Add(chroma.Comment, "italic #888888")
	builder.Add(chroma.Number, "#ffff00")
	style, err := builder.Build()
	require.NoError(t, err)

	tokens := []chroma.Token{
		{Type: chroma.Keyword, Value: "func"},
		{Type: chroma.Text, Value: " "},
		{Type: chroma.String, Value: `"hello"`},
		{Type: chroma.Text, Value: " "},
		{Type: chroma.Comment, Value: "// comment"},
		{Type: chroma.Text, Value: " "},
		{Type: chroma.Number, Value: "42"},
	}
	it := tokenIterator(tokens)

	var buf bytes.Buffer

	err = f.Format(&buf, style, it)
	require.NoError(t, err)

	out := buf.String()
	assert.Contains(t, out, "func")
	assert.Contains(t, out, `"hello"`)
	assert.Contains(t, out, "// comment")
	assert.Contains(t, out, "42")
	assert.Contains(t, out, "\x1b[", "expected ANSI escape codes in output")
}

func TestNewFormatter_ZeroEntryPassthrough(t *testing.T) {
	f := NewFormatter()

	builder := chromastyles.Fallback.Builder()
	style, err := builder.Build()
	require.NoError(t, err)

	tokens := []chroma.Token{
		{Type: chroma.Name, Value: "myFunc"},
	}
	it := tokenIterator(tokens)

	var buf bytes.Buffer

	err = f.Format(&buf, style, it)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "myFunc")
}

func tokenIterator(tokens []chroma.Token) func() chroma.Token {
	i := 0

	return func() chroma.Token {
		if i >= len(tokens) {
			return chroma.EOF
		}

		t := tokens[i]
		i++

		return t
	}
}
