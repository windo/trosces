package main

import (
	"image/color"
	"log"
	"sync"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

type Header struct {
	min    int
	max    int
	active []int

	keyWidth    float32
	keyHeight   float32
	borderWidth float32

	blackColor   color.Color
	blackHiColor color.Color
	whiteColor   color.Color
	whiteHiColor color.Color
	borderColor  color.Color

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

		whiteColor:   color.RGBA{0xb7, 0x9a, 0x9a, 0xff},
		whiteHiColor: color.RGBA{0xfd, 0xe5, 0xe5, 0xff},
		blackColor:   color.RGBA{0x18, 0x11, 0x11, 0xff},
		blackHiColor: color.RGBA{0x59, 0x44, 0x44, 0xff},
		borderColor:  color.Black,

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

func (header *Header) Draw(image *ebiten.Image, op *ebiten.DrawImageOptions) {
	if header.max-header.min == 0 {
		return
	}
	image.DrawImage(header.getBase(), op)
	image.DrawImage(header.getOverlay(), op)
}

func (header *Header) Width() float32 {
	return float32(header.max-header.min+1) * header.keyWidth
}

// Internal

func (header *Header) drawKey(image *ebiten.Image, note int, keyColor color.Color) {
	halfBorder := header.borderWidth / 2
	halfWidth := header.keyWidth / 2
	halfHeight := header.keyHeight / 2

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
			path.LineTo(extraOffset, halfHeight-halfBorder)
			path.LineTo(keyOffset, halfHeight-halfBorder)
			path.LineTo(keyOffset, header.keyHeight-header.borderWidth)
		} else {
			path.MoveTo(keyOffset, header.borderWidth)
			path.LineTo(keyOffset, header.keyHeight-header.borderWidth)
		}
		if rightBlack {
			extraOffset := keyEndOffset + halfWidth
			path.LineTo(keyEndOffset, header.keyHeight-header.borderWidth)
			path.LineTo(keyEndOffset, halfHeight-halfBorder)
			path.LineTo(extraOffset, halfHeight-halfBorder)
			path.LineTo(extraOffset, header.borderWidth)
		} else {
			path.LineTo(keyEndOffset, header.keyHeight-header.borderWidth)
			path.LineTo(keyEndOffset, header.borderWidth)
		}
	} else {
		path.MoveTo(keyOffset, halfHeight+halfBorder)
		path.LineTo(keyOffset, header.keyHeight-header.borderWidth)
		path.LineTo(keyEndOffset, header.keyHeight-header.borderWidth)
		path.LineTo(keyEndOffset, halfHeight+halfBorder)
	}
	op := vector.FillOptions{Color: keyColor}
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
			var keyColor color.Color
			if Note(note).IsWhite() {
				keyColor = header.whiteColor
			} else {
				keyColor = header.blackColor
			}
			header.drawKey(header.base, note, keyColor)
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

		for _, note := range header.active {
			var keyColor color.Color
			if Note(note).IsWhite() {
				keyColor = header.whiteHiColor
			} else {
				keyColor = header.blackHiColor
			}
			header.drawKey(header.overlay, note, keyColor)
		}

		header.overlayReady = true
	}
	return header.overlay
}
