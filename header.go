package main

import (
	"image/color"
	"log"
	"sync"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

type Header struct {
	min                int
	max                int
	active             []int
	highlight          []int
	highlightDelivered bool

	keyboard    bool
	keyWidth    float32
	keyHeight   float32
	borderWidth float32

	blackColor          color.Color
	blackActiveColor    color.Color
	blackHighlightColor color.Color
	whiteColor          color.Color
	whiteActiveColor    color.Color
	whiteHighlightColor color.Color
	borderColor         color.Color

	base         *ebiten.Image
	overlay      *ebiten.Image
	overlayReady bool

	mu sync.Mutex
}

func NewHeader(keyWidth float32, keyHeight float32) *Header {
	return &Header{
		keyWidth:    keyWidth,
		keyHeight:   keyHeight,
		borderWidth: 2,

		whiteColor:          color.RGBA{0xb7, 0x9a, 0x9a, 0xff},
		whiteHighlightColor: color.RGBA{0xda, 0xbf, 0xbf, 0xff},
		whiteActiveColor:    color.RGBA{0xfd, 0xe5, 0xe5, 0xff},
		blackColor:          color.RGBA{0x18, 0x11, 0x11, 0xff},
		blackHighlightColor: color.RGBA{0x38, 0x2a, 0x2a, 0xff},
		blackActiveColor:    color.RGBA{0x59, 0x44, 0x44, 0xff},
		borderColor:         color.Black,

		active: []int{},
	}
}

func (header *Header) SetRange(min int, max int) {
	header.mu.Lock()
	defer header.mu.Unlock()

	if header.min != min {
		header.min = min
		header.base = nil
		header.overlay = nil
		header.overlayReady = false
	}
	if header.max != max {
		header.max = max
		header.base = nil
		header.overlay = nil
		header.overlayReady = false
	}
}

func (header *Header) SetActive(active []int) {
	header.mu.Lock()
	defer header.mu.Unlock()

	var changed bool
	if len(active) != len(header.active) {
		changed = true
	} else {
		for i := range header.active {
			if header.active[i] != active[i] {
				changed = true
				break
			}
		}
	}
	if changed {
		header.active = make([]int, len(active))
		copy(header.active, active)
		header.overlayReady = false
	}
}

func (header *Header) SetHighlight(highlight []int) {
	header.mu.Lock()
	defer header.mu.Unlock()

	var changed bool
	if len(highlight) != len(header.highlight) {
		changed = true
	} else {
		for i := range header.highlight {
			if header.highlight[i] != highlight[i] {
				changed = true
				break
			}
		}
	}
	if changed {
		header.highlight = make([]int, len(highlight))
		copy(header.highlight, highlight)
		header.overlayReady = false
		header.highlightDelivered = false
	}
}

func (header *Header) GetUpdatedHighlight() []int {
	header.mu.Lock()
	defer header.mu.Unlock()

	if !header.highlightDelivered {
		header.highlightDelivered = true
		return header.highlight
	}
	return nil
}

func (header *Header) Draw(image *ebiten.Image, op *ebiten.DrawImageOptions) {
	image.DrawImage(header.getBase(), op)
	image.DrawImage(header.getOverlay(), op)
}

func (header *Header) Width() float32 {
	return float32(header.max-header.min+1) * header.keyWidth
}

// Internal

func (header *Header) drawKey(image *ebiten.Image, note int, active, highlight bool) {
	halfBorder := header.borderWidth / 2
	halfWidth := header.keyWidth / 2
	blackHeight := header.keyHeight * 0.25

	baseOffset := float32(note-header.min) * header.keyWidth
	keyOffset := baseOffset + halfBorder
	keyEndOffset := baseOffset + header.keyWidth - halfBorder
	leftBlack := !Note(note - 1).IsWhite()
	rightBlack := !Note(note + 1).IsWhite()

	if note == header.min {
		keyOffset += halfBorder
		leftBlack = false
	}
	if note == header.max {
		keyEndOffset -= halfBorder
		rightBlack = false
	}

	path := vector.Path{}
	if Note(note).IsWhite() {
		if leftBlack {
			extraOffset := keyOffset - halfWidth
			path.MoveTo(extraOffset, header.borderWidth)
			path.LineTo(extraOffset, blackHeight-halfBorder)
			path.LineTo(keyOffset, blackHeight-halfBorder)
			path.LineTo(keyOffset, header.keyHeight-header.borderWidth)
		} else {
			path.MoveTo(keyOffset, header.borderWidth)
			path.LineTo(keyOffset, header.keyHeight-header.borderWidth)
		}
		if rightBlack {
			extraOffset := keyEndOffset + halfWidth
			path.LineTo(keyEndOffset, header.keyHeight-header.borderWidth)
			path.LineTo(keyEndOffset, blackHeight-halfBorder)
			path.LineTo(extraOffset, blackHeight-halfBorder)
			path.LineTo(extraOffset, header.borderWidth)
		} else {
			path.LineTo(keyEndOffset, header.keyHeight-header.borderWidth)
			path.LineTo(keyEndOffset, header.borderWidth)
		}
	} else {
		path.MoveTo(keyOffset, blackHeight+halfBorder)
		path.LineTo(keyOffset, header.keyHeight-header.borderWidth)
		path.LineTo(keyEndOffset, header.keyHeight-header.borderWidth)
		path.LineTo(keyEndOffset, blackHeight+halfBorder)
	}
	var keyColor color.Color
	if Note(note).IsWhite() {
		if active {
			keyColor = header.whiteActiveColor
		} else if highlight {
			keyColor = header.whiteHighlightColor
		} else {
			keyColor = header.whiteColor
		}
	} else {
		if active {
			keyColor = header.blackActiveColor
		} else if highlight {
			keyColor = header.blackHighlightColor
		} else {
			keyColor = header.blackColor
		}
	}
	op := vector.FillOptions{Color: keyColor}
	path.Fill(image, &op)
}

func (header *Header) drawPad(image *ebiten.Image, pos int, active bool) {
	halfBorder := header.borderWidth / 2
	baseOffset := float32(pos-header.min) * header.keyWidth
	keyOffset := baseOffset + halfBorder
	keyEndOffset := baseOffset + header.keyWidth - halfBorder

	path := vector.Path{}
	path.MoveTo(keyOffset, header.borderWidth)
	path.LineTo(keyOffset, header.keyHeight-header.borderWidth)
	path.LineTo(keyEndOffset, header.keyHeight-header.borderWidth)
	path.LineTo(keyEndOffset, header.borderWidth)
	var padColor color.Color
	if active {
		padColor = header.whiteActiveColor
	} else {
		padColor = header.whiteColor
	}
	op := vector.FillOptions{Color: padColor}
	path.Fill(image, &op)
}

func (header *Header) getBase() *ebiten.Image {
	header.mu.Lock()
	defer header.mu.Unlock()

	if header.base == nil {
		log.Printf("New header base image")
		header.base = ebiten.NewImage(
			int(header.keyWidth*float32(header.max-header.min+1)),
			int(header.keyHeight),
		)
		header.base.Fill(header.borderColor)

		for note := header.min; note <= header.max; note++ {
			if header.keyboard {
				header.drawKey(header.base, note, false, false)
			} else {
				header.drawPad(header.base, note, false)
			}
		}
	}
	return header.base
}

func (header *Header) getOverlay() *ebiten.Image {
	header.mu.Lock()
	defer header.mu.Unlock()

	if !header.overlayReady {
		if header.overlay == nil {
			header.overlay = ebiten.NewImage(
				int(header.keyWidth*float32(header.max-header.min+1)),
				int(header.keyHeight),
			)
		}
		header.overlay.Fill(color.Transparent)

		for _, note := range header.highlight {
			if header.keyboard {
				header.drawKey(header.overlay, note, false, true)
			} else {
				log.Printf("Pads have no highlighting!")
			}
		}

		for _, note := range header.active {
			if header.keyboard {
				header.drawKey(header.overlay, note, true, false)
			} else {
				header.drawPad(header.overlay, note, true)
			}
		}

		header.overlayReady = true
	}
	return header.overlay
}
