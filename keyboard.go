package main

import (
	"fmt"
	"image/color"
	"strconv"
	"sync"
	"unicode"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

type Note int

func (n Note) IsWhite() bool {
	degree := int(n) % 12
	return map[int]bool{0: true, 2: true, 4: true, 5: true, 7: true, 9: true, 11: true}[degree]
}

func NewNote(noteStr string) (Note, error) {
	i := 0
	note := Note(0)
	runes := []rune(noteStr)
	if len(runes) < 2 {
		return 0, fmt.Errorf("too short")
	}
	degree := runes[i]
	if !unicode.IsLetter(degree) {
		return 0, fmt.Errorf("degree not a letter")
	}
	degree = unicode.ToLower(degree)
	if !map[rune]bool{'a': true, 'b': true, 'c': true, 'd': true, 'e': true, 'f': true, 'g': true}[degree] {
		return 0, fmt.Errorf("invalid degree")
	}
	note = map[rune]Note{'c': 0, 'd': 2, 'e': 4, 'f': 5, 'g': 7, 'a': 9, 'b': 11}[degree]
	i++
	if !unicode.IsDigit(runes[i]) {
		sharpFlat := runes[i]
		if unicode.IsLetter(sharpFlat) {
			sharpFlat = unicode.ToLower(sharpFlat)
		}
		switch sharpFlat {
		case 's':
			note++
		case '#':
			note++
		case 'f':
			note--
		case 'b':
			note--
		default:
			return 0, fmt.Errorf("invalid sharp/flat")
		}
		i++
	}
	octave, err := strconv.Atoi(string(runes[i:]))
	if err != nil {
		return 0, err
	}
	note += Note(12 * octave)

	return note, nil
}

type Keyboard struct {
	min Note
	max Note

	keyWidth    float32
	keyHeight   float32
	borderWidth float32

	blackColor  color.Color
	whiteColor  color.Color
	borderColor color.Color

	cached *ebiten.Image

	mu sync.Mutex
}

func NewKeyboard(keyWidth float32, keyHeight float32) *Keyboard {
	return &Keyboard{
		keyWidth:    keyWidth,
		keyHeight:   keyHeight,
		borderWidth: 2,

		whiteColor:  color.RGBA{0xfd, 0xe5, 0xe5, 0xff},
		blackColor:  color.RGBA{0x2e, 0x21, 0x21, 0xff},
		borderColor: color.Black,
	}
}

func (kb *Keyboard) SetRange(min Note, max Note) {
	kb.mu.Lock()
	defer kb.mu.Unlock()

	if kb.min != min {
		kb.min = min
		kb.cached = nil
	}
	if kb.max != max {
		kb.max = max
		kb.cached = nil
	}
}

func (kb *Keyboard) getCached() *ebiten.Image {
	kb.mu.Lock()
	defer kb.mu.Unlock()

	if kb.cached == nil {
		kb.cached = ebiten.NewImage(
			int(kb.keyWidth*float32(kb.max-kb.min+1)),
			int(kb.keyHeight),
		)

		kb.cached.Fill(kb.borderColor)

		whiteOp := &vector.FillOptions{
			Color: kb.whiteColor,
		}
		blackOp := &vector.FillOptions{
			Color: kb.blackColor,
		}

		halfBorder := kb.borderWidth / 2
		halfWidth := kb.keyWidth / 2
		halfHeight := kb.keyHeight / 2

		for note := kb.min; note <= kb.max; note++ {
			baseOffset := float32(note-kb.min) * kb.keyWidth
			keyOffset := baseOffset + halfBorder
			keyEndOffset := baseOffset + kb.keyWidth - halfBorder
			leftBlack := !(note - 1).IsWhite()
			rightBlack := !(note + 1).IsWhite()
			if note == kb.min {
				keyOffset += halfBorder
				leftBlack = false
			}
			if note == kb.max {
				keyEndOffset -= halfBorder
				rightBlack = false
			}

			path := vector.Path{}
			if note.IsWhite() {
				if leftBlack {
					extraOffset := keyOffset - halfWidth
					path.MoveTo(extraOffset, kb.borderWidth)
					path.LineTo(extraOffset, halfHeight-halfBorder)
					path.LineTo(keyOffset, halfHeight-halfBorder)
					path.LineTo(keyOffset, kb.keyHeight-kb.borderWidth)
				} else {
					path.MoveTo(keyOffset, kb.borderWidth)
					path.LineTo(keyOffset, kb.keyHeight-kb.borderWidth)
				}
				if rightBlack {
					extraOffset := keyEndOffset + halfWidth
					path.LineTo(keyEndOffset, kb.keyHeight-kb.borderWidth)
					path.LineTo(keyEndOffset, halfHeight-halfBorder)
					path.LineTo(extraOffset, halfHeight-halfBorder)
					path.LineTo(extraOffset, kb.borderWidth)
				} else {
					path.LineTo(keyEndOffset, kb.keyHeight-kb.borderWidth)
					path.LineTo(keyEndOffset, kb.borderWidth)
				}
				path.Fill(kb.cached, whiteOp)
			} else {
				path.MoveTo(keyOffset, halfHeight+halfBorder)
				path.LineTo(keyOffset, kb.keyHeight-kb.borderWidth)
				path.LineTo(keyEndOffset, kb.keyHeight-kb.borderWidth)
				path.LineTo(keyEndOffset, halfHeight+halfBorder)
				path.Fill(kb.cached, blackOp)
			}
		}
	}
	return kb.cached
}

func (kb *Keyboard) Draw(image *ebiten.Image, op *ebiten.DrawImageOptions) {
	image.DrawImage(kb.getCached(), op)
}
