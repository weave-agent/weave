package tui

import (
	"testing"

	uv "github.com/charmbracelet/ultraviolet"
	"github.com/stretchr/testify/assert"
)

func TestComposer_Draw_DelegatesToComponents(t *testing.T) {
	c := NewComposer()

	headerDrawn := false
	mainDrawn := false
	pillsDrawn := false
	editorDrawn := false
	footerDrawn := false

	header := DrawableFunc(func(_ uv.Screen, _ uv.Rectangle) { headerDrawn = true })
	main := DrawableFunc(func(_ uv.Screen, _ uv.Rectangle) { mainDrawn = true })
	pills := DrawableFunc(func(_ uv.Screen, _ uv.Rectangle) { pillsDrawn = true })
	editor := DrawableFunc(func(_ uv.Screen, _ uv.Rectangle) { editorDrawn = true })
	footer := DrawableFunc(func(_ uv.Screen, _ uv.Rectangle) { footerDrawn = true })

	scr := uv.NewScreenBuffer(120, 40)
	opts := DrawOpts{EditorLines: 3, HeaderRows: 1, PillRows: 1}

	c.Draw(scr, header, main, pills, editor, footer, opts)

	assert.True(t, headerDrawn, "header should be drawn")
	assert.True(t, mainDrawn, "main should be drawn")
	assert.True(t, pillsDrawn, "pills should be drawn")
	assert.True(t, editorDrawn, "editor should be drawn")
	assert.True(t, footerDrawn, "footer should be drawn")
}

func TestComposer_Draw_SkipsNilComponents(t *testing.T) {
	c := NewComposer()

	mainDrawn := false
	footerDrawn := false

	main := DrawableFunc(func(_ uv.Screen, _ uv.Rectangle) { mainDrawn = true })
	footer := DrawableFunc(func(_ uv.Screen, _ uv.Rectangle) { footerDrawn = true })

	scr := uv.NewScreenBuffer(120, 40)
	opts := DrawOpts{EditorLines: 3}

	c.Draw(scr, nil, main, nil, nil, footer, opts)

	// Main and footer are both drawn because the layout allocates space for them
	// even when other components (header, pills, editor) are nil
	assert.True(t, mainDrawn, "main should be drawn (layout allocates space)")
	assert.True(t, footerDrawn, "footer should be drawn")
}

func TestComposer_Draw_SkipsEmptyRegions(t *testing.T) {
	c := NewComposer()

	// Without header/pills, header and pills have 0-size areas
	headerDrawn := false
	pillsDrawn := false

	header := DrawableFunc(func(_ uv.Screen, _ uv.Rectangle) { headerDrawn = true })
	pills := DrawableFunc(func(_ uv.Screen, _ uv.Rectangle) { pillsDrawn = true })

	scr := uv.NewScreenBuffer(120, 40)
	opts := DrawOpts{EditorLines: 3} // no header or pills

	c.Draw(scr, header, nil, pills, nil, nil, opts)

	assert.False(t, headerDrawn, "header should be skipped (0 rows)")
	assert.False(t, pillsDrawn, "pills should be skipped (0 rows)")
}

func TestDrawableFunc(t *testing.T) {
	called := false
	f := DrawableFunc(func(_ uv.Screen, _ uv.Rectangle) { called = true })

	scr := uv.NewScreenBuffer(80, 24)
	f.Draw(scr, uv.Rect(0, 0, 80, 24))

	assert.True(t, called, "DrawableFunc should call the wrapped function")
}

func TestComposer_Draw_LayoutMatchesCompute(t *testing.T) {
	c := NewComposer()

	var mainArea uv.Rectangle

	main := DrawableFunc(func(_ uv.Screen, area uv.Rectangle) { mainArea = area })

	scr := uv.NewScreenBuffer(120, 40)
	opts := DrawOpts{EditorLines: 3, HeaderRows: 1, PillRows: 1}

	c.Draw(scr, nil, main, nil, nil, nil, opts)

	// Main area should match what LayoutEngine computes
	expected := c.Engine.ComputeFull(120, 40, 3, 1, 1)
	assert.Equal(t, expected.Main, mainArea, "main area should match layout computation")
}
