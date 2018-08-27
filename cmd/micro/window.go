package main

import (
	"strconv"
	"unicode/utf8"

	runewidth "github.com/mattn/go-runewidth"
	"github.com/zyedidia/micro/cmd/micro/buffer"
	"github.com/zyedidia/micro/cmd/micro/config"
	"github.com/zyedidia/micro/cmd/micro/screen"
	"github.com/zyedidia/micro/cmd/micro/util"
	"github.com/zyedidia/tcell"
)

type Window struct {
	// X and Y coordinates for the top left of the window
	X int
	Y int

	// Width and Height for the window
	Width  int
	Height int

	// Which line in the buffer to start displaying at (vertical scroll)
	StartLine int
	// Which visual column in the to start displaying at (horizontal scroll)
	StartCol int

	// Buffer being shown in this window
	Buf *buffer.Buffer

	sline *StatusLine
}

func NewWindow(x, y, width, height int, buf *buffer.Buffer) *Window {
	w := new(Window)
	w.X, w.Y, w.Width, w.Height, w.Buf = x, y, width, height, buf

	w.sline = NewStatusLine(w)

	return w
}

func (w *Window) Clear() {
	for y := 0; y < w.Height; y++ {
		for x := 0; x < w.Width; x++ {
			screen.Screen.SetContent(w.X+x, w.Y+y, ' ', nil, config.DefStyle)
		}
	}
}

func (w *Window) DrawLineNum(lineNumStyle tcell.Style, softwrapped bool, maxLineNumLength int, vloc *buffer.Loc, bloc *buffer.Loc) {
	lineNum := strconv.Itoa(bloc.Y + 1)

	// Write the spaces before the line number if necessary
	for i := 0; i < maxLineNumLength-len(lineNum); i++ {
		screen.Screen.SetContent(w.X+vloc.X, w.Y+vloc.Y, ' ', nil, lineNumStyle)
		vloc.X++
	}
	// Write the actual line number
	for _, ch := range lineNum {
		if softwrapped {
			screen.Screen.SetContent(w.X+vloc.X, w.Y+vloc.Y, ' ', nil, lineNumStyle)
		} else {
			screen.Screen.SetContent(w.X+vloc.X, w.Y+vloc.Y, ch, nil, lineNumStyle)
		}
		vloc.X++
	}

	// Write the extra space
	screen.Screen.SetContent(w.X+vloc.X, w.Y+vloc.Y, ' ', nil, lineNumStyle)
	vloc.X++
}

// GetStyle returns the highlight style for the given character position
// If there is no change to the current highlight style it just returns that
func (w *Window) GetStyle(style tcell.Style, bloc buffer.Loc, r rune) tcell.Style {
	if group, ok := w.Buf.Match(bloc.Y)[bloc.X]; ok {
		s := config.GetColor(group.String())
		return s
	}
	return style
}

func (w *Window) ShowCursor(x, y int, main bool) {
	if main {
		screen.Screen.ShowCursor(x, y)
	} else {
		r, _, _, _ := screen.Screen.GetContent(x, y)
		screen.Screen.SetContent(x, y, r, nil, config.DefStyle.Reverse(true))
	}
}

// DisplayBuffer draws the buffer being shown in this window on the screen.Screen
func (w *Window) DisplayBuffer() {
	b := w.Buf

	bufHeight := w.Height
	if b.Settings["statusline"].(bool) {
		bufHeight--
	}

	// TODO: Rehighlighting
	// start := w.StartLine
	if b.Settings["syntax"].(bool) && b.SyntaxDef != nil {
		// 	if start > 0 && b.lines[start-1].rehighlight {
		// 		b.highlighter.ReHighlightLine(b, start-1)
		// 		b.lines[start-1].rehighlight = false
		// 	}
		//
		// 	b.highlighter.ReHighlightStates(b, start)
		//
		b.Highlighter.HighlightMatches(b, w.StartLine, w.StartLine+bufHeight)
	}

	lineNumStyle := config.DefStyle
	if style, ok := config.Colorscheme["line-number"]; ok {
		lineNumStyle = style
	}

	// We need to know the string length of the largest line number
	// so we can pad appropriately when displaying line numbers
	maxLineNumLength := len(strconv.Itoa(b.LinesNum()))

	tabsize := int(b.Settings["tabsize"].(float64))
	softwrap := b.Settings["softwrap"].(bool)

	// this represents the current draw position
	// within the current window
	vloc := buffer.Loc{0, 0}

	// this represents the current draw position in the buffer (char positions)
	bloc := buffer.Loc{w.StartCol, w.StartLine}

	curStyle := config.DefStyle
	for vloc.Y = 0; vloc.Y < bufHeight; vloc.Y++ {
		vloc.X = 0
		if b.Settings["ruler"].(bool) {
			w.DrawLineNum(lineNumStyle, false, maxLineNumLength, &vloc, &bloc)
		}

		line := b.LineBytes(bloc.Y)
		line, nColsBeforeStart := util.SliceVisualEnd(line, bloc.X, tabsize)
		totalwidth := bloc.X - nColsBeforeStart
		for len(line) > 0 {
			if w.Buf.GetActiveCursor().X == bloc.X && w.Buf.GetActiveCursor().Y == bloc.Y {
				w.ShowCursor(vloc.X, vloc.Y, true)
			}

			r, size := utf8.DecodeRune(line)

			curStyle = w.GetStyle(curStyle, bloc, r)

			if nColsBeforeStart <= 0 {
				screen.Screen.SetContent(w.X+vloc.X, w.Y+vloc.Y, r, nil, curStyle)
				vloc.X++
			}
			nColsBeforeStart--

			width := 0

			char := ' '
			switch r {
			case '\t':
				ts := tabsize - (totalwidth % tabsize)
				width = ts
			default:
				width = runewidth.RuneWidth(r)
				char = '@'
			}

			bloc.X++
			line = line[size:]

			// Draw any extra characters either spaces for tabs or @ for incomplete wide runes
			if width > 1 {
				for i := 1; i < width; i++ {
					if nColsBeforeStart <= 0 {
						screen.Screen.SetContent(w.X+vloc.X, w.Y+vloc.Y, char, nil, curStyle)
						vloc.X++
					}
					nColsBeforeStart--
				}
			}
			totalwidth += width

			// If we reach the end of the window then we either stop or we wrap for softwrap
			if vloc.X >= w.Width {
				if !softwrap {
					break
				} else {
					vloc.Y++
					if vloc.Y >= bufHeight {
						break
					}
					vloc.X = 0
					// This will draw an empty line number because the current line is wrapped
					w.DrawLineNum(lineNumStyle, true, maxLineNumLength, &vloc, &bloc)
				}
			}
		}
		if w.Buf.GetActiveCursor().X == bloc.X && w.Buf.GetActiveCursor().Y == bloc.Y {
			w.ShowCursor(vloc.X, vloc.Y, true)
		}
		bloc.X = w.StartCol
		bloc.Y++
		if bloc.Y >= b.LinesNum() {
			break
		}
	}
}

func (w *Window) DisplayStatusLine() {
	w.sline.Display()
}
