package main

import (
	"context"
	"errors"
	"image/color"
	"log"
	"runtime/trace"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

type Track struct {
	header *Header
	trail  *Trail
	mapper *Mapper
}

func (track *Track) Resolve() {
	track.header.SetRange(track.trail.minPos, track.trail.maxPos)
	track.header.SetActive(track.trail.ActivePos())
}

func (track *Track) Draw(ctx context.Context, image *ebiten.Image, op *ebiten.DrawImageOptions) {
	trailOp := *op
	trailOp.GeoM.Translate(0, float64(track.header.keyHeight))
	track.trail.Draw(ctx, image, &trailOp)
	track.header.Draw(image, op)
}

func (track *Track) Width() float32 {
	return track.header.Width()
}

var Finished = errors.New("Trosces finished")

type Mapper struct {
	nameToId map[string]int
	nextId   int
}

func NewMapper() *Mapper {
	return &Mapper{
		nameToId: map[string]int{},
		nextId:   0,
	}
}

func (m *Mapper) Get(name string) int {
	if i, ok := m.nameToId[name]; ok {
		return i
	} else {
		m.nameToId[name] = m.nextId
		m.nextId++
		return m.nameToId[name]
	}
}

type Trosces struct {
	keyboard *Track
	drums    *Track
	layers   *Track

	variantMappers map[int]*Mapper
}

func NewTrosces() *Trosces {
	log.Printf("Creating new Trosces")
	trosces := &Trosces{
		keyboard: &Track{
			header: NewHeader(15, 30),
			trail:  NewTrail(time.Second, 4*time.Second, 256, 15),
			mapper: NewMapper(),
		},
		drums: &Track{
			header: NewHeader(30, 30),
			trail:  NewTrail(time.Second, 4*time.Second, 256, 30),
			mapper: NewMapper(),
		},
		layers: &Track{
			header: NewHeader(30, 30),
			trail:  NewTrail(8*time.Second, 120*time.Second, 8, 30),
			mapper: NewMapper(),
		},
		variantMappers: map[int]*Mapper{},
	}
	trosces.keyboard.header.keyboard = true
	trosces.drums.header.borderWidth = 2
	trosces.drums.trail.borderWidth = 2
	trosces.layers.header.borderWidth = 2
	trosces.layers.trail.borderWidth = 2

	return trosces
}

// Events from OSC.

func (trosces *Trosces) PlayNote(instrument string, note Note, duration time.Duration) {
	iNum := trosces.keyboard.mapper.Get(instrument)
	if duration == 0 {
		// "forever"
		duration = 24 * time.Hour
	}
	trosces.keyboard.trail.Span(iNum, int(note), duration)
}

func (trosces *Trosces) PlayDrum(instrument string, duration time.Duration) {
	iNum := trosces.drums.mapper.Get(instrument)
	if duration == 0 {
		// short hit
		duration = time.Second / 8
	}
	trosces.drums.trail.Span(iNum, iNum, duration)
}

func (trosces *Trosces) PlayLayer(name string, duration time.Duration, variant string) {
	lNum := trosces.layers.mapper.Get(name)
	if _, ok := trosces.variantMappers[lNum]; !ok {
		trosces.variantMappers[lNum] = NewMapper()
	}
	vNum := trosces.variantMappers[lNum].Get(variant)
	trosces.layers.trail.Span(vNum, lNum, duration)
}

func (trosces *Trosces) Sync(bpm int) {
	// TODO: unimplemented
}

// Implements ebiten.Game interface.

func (trosces *Trosces) Update() error {
	_, task := trace.NewTask(context.Background(), "UpdateTrosces")
	defer task.End()

	// Header matches the trail.
	trosces.keyboard.Resolve()
	trosces.drums.Resolve()
	trosces.layers.Resolve()

	// Grid steps
	if inpututil.IsKeyJustPressed(ebiten.Key3) {
		trosces.keyboard.trail.SetGridSteps(3)
		trosces.drums.trail.SetGridSteps(3)
	}
	if inpututil.IsKeyJustPressed(ebiten.Key4) {
		trosces.keyboard.trail.SetGridSteps(4)
		trosces.drums.trail.SetGridSteps(4)
	}

	// Maybe finish.
	if inpututil.IsKeyJustPressed(ebiten.KeyQ) || inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
		return Finished
	}

	return nil
}

func (trosces *Trosces) Draw(screen *ebiten.Image) {
	ctx, task := trace.NewTask(context.Background(), "DrawTrosces")
	defer task.End()

	var x float64
	op := ebiten.DrawImageOptions{}
	trosces.keyboard.Draw(ctx, screen, &op)
	x += float64(trosces.keyboard.Width())

	op = ebiten.DrawImageOptions{}
	op.GeoM.Translate(x, 0)
	trosces.drums.Draw(ctx, screen, &op)
	x += float64(trosces.drums.Width())

	op = ebiten.DrawImageOptions{}
	op.GeoM.Translate(x, 0)
	trosces.layers.Draw(ctx, screen, &op)
	x += float64(trosces.layers.Width())

	// TODO: Actually don't draw the extra pixels above!
	path := vector.Path{}
	line := trosces.keyboard.header.keyHeight + float32(trosces.keyboard.trail.length.Seconds())*trosces.keyboard.trail.secondSize
	path.MoveTo(0, line-256)
	path.LineTo(float32(x), line-256)
	path.LineTo(float32(x), line+256)
	path.LineTo(0, line+256)
	path.Fill(screen, &vector.FillOptions{Color: color.Black})
}

func (trosces *Trosces) Layout(outsideWidth, outsideHeight int) (int, int) {
	return outsideWidth, outsideHeight
}
