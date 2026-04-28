package xchroma

import (
	"bytes"
	"image/color"
	"strings"
	"testing"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
	chromastyles "github.com/alecthomas/chroma/v2/styles"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewFormatter_NilBackground(t *testing.T) {
	f := NewFormatter(nil)
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

func TestNewFormatter_WithBackground(t *testing.T) {
	bg := color.RGBA{R: 40, G: 40, B: 40, A: 255}
	f := NewFormatter(bg)
	require.NotNil(t, f)

	style := chromastyles.Fallback
	lexer := lexers.Get("go")
	require.NotNil(t, lexer)

	it, err := lexer.Tokenise(nil, `var x = 42`)
	require.NoError(t, err)

	var buf bytes.Buffer
	err = f.Format(&buf, style, it)
	require.NoError(t, err)

	out := buf.String()
	assert.Contains(t, out, "var")
	assert.Contains(t, out, "42")
}

func TestNewFormatter_EmptyInput(t *testing.T) {
	f := NewFormatter(nil)
	style := chromastyles.Fallback

	it := func() chroma.Token { return chroma.EOF }

	var buf bytes.Buffer
	err := f.Format(&buf, style, it)
	require.NoError(t, err)
	assert.Empty(t, buf.String())
}

func TestNewFormatter_PlainText(t *testing.T) {
	f := NewFormatter(nil)
	style := chromastyles.Fallback

	tokens := []chroma.Token{
		{Type: chroma.Text, Value: "hello world"},
	}
	it := tokenIterator(tokens)

	var buf bytes.Buffer
	err := f.Format(&buf, style, it)
	require.NoError(t, err)
	// Text tokens get styled by the fallback style, so just check the value is present
	assert.Contains(t, buf.String(), "hello world")
}

func TestNewFormatter_BoldToken(t *testing.T) {
	f := NewFormatter(nil)

	// Create a style with a bold keyword entry
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
	// Bold keyword should produce longer output due to ANSI codes
	assert.Greater(t, len(out), len("func main"))
}

func TestNewFormatter_MultipleTokenTypes(t *testing.T) {
	f := NewFormatter(nil)

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

	// Output should have ANSI escape sequences for styling
	assert.True(t, strings.Contains(out, "\x1b["), "expected ANSI escape codes in output")
}

func TestNewFormatter_ZeroEntryPassthrough(t *testing.T) {
	f := NewFormatter(nil)

	// Use a minimal style where token type has no entry
	builder := chromastyles.Fallback.Builder()
	// Don't add any entries for Name — it should fall through to zero entry
	style, err := builder.Build()
	require.NoError(t, err)

	tokens := []chroma.Token{
		{Type: chroma.Name, Value: "myFunc"},
	}
	it := tokenIterator(tokens)

	var buf bytes.Buffer
	err = f.Format(&buf, style, it)
	require.NoError(t, err)

	// Name tokens may or may not have style entries in fallback,
	// but the value should always appear in output
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
