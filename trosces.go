package main

import (
	"context"
	"errors"
	"image/color"
	"log"
	"runtime/trace"

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
	pulse *Pulse

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
			trail:  NewTrail(Beats(1), Beats(4), 192, 15),
			mapper: NewMapper(),
		},
		drums: &Track{
			header: NewHeader(30, 30),
			trail:  NewTrail(Beats(1), Beats(4), 192, 30),
			mapper: NewMapper(),
		},
		layers: &Track{
			header: NewHeader(30, 30),
			trail:  NewTrail(Beats(16), Beats(128), 8, 30),
			mapper: NewMapper(),
		},
		variantMappers: map[int]*Mapper{},

		pulse: NewPulse(60),
	}
	trosces.keyboard.header.keyboard = true
	trosces.drums.header.borderWidth = 2
	trosces.drums.trail.borderWidth = 2
	trosces.layers.header.borderWidth = 2
	trosces.layers.trail.borderWidth = 2

	trosces.keyboard.trail.pulse = trosces.pulse
	trosces.drums.trail.pulse = trosces.pulse
	trosces.layers.trail.pulse = trosces.pulse

	return trosces
}

// Events from OSC.

func (trosces *Trosces) PlayNote(instrument string, note Note, duration Duration) {
	iNum := trosces.keyboard.mapper.Get(instrument)
	if duration.IsZero() {
		duration = Forever()
	}
	trosces.keyboard.trail.Span(iNum, int(note), duration)
}

func (trosces *Trosces) StopNote(instrument string, note Note) {
	iNum := trosces.keyboard.mapper.Get(instrument)
	trosces.keyboard.trail.Stop(iNum, int(note))
}

func (trosces *Trosces) PlayDrum(instrument string, duration Duration) {
	iNum := trosces.drums.mapper.Get(instrument)
	if duration.IsZero() {
		duration = Beats(1.0 / 8)
	}
	trosces.drums.trail.Span(iNum, iNum, duration)
}

func (trosces *Trosces) PlayLayer(name string, duration Duration, variant string) {
	lNum := trosces.layers.mapper.Get(name)
	if _, ok := trosces.variantMappers[lNum]; !ok {
		trosces.variantMappers[lNum] = NewMapper()
	}
	vNum := trosces.variantMappers[lNum].Get(variant)
	trosces.layers.trail.Span(vNum, lNum, duration)
}

func (trosces *Trosces) Sync(bpm int) {
	trosces.pulse.Sync(float32(bpm))
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

	// Freeze
	if inpututil.IsKeyJustPressed(ebiten.KeySpace) {
		trosces.pulse.ToggleFrozen()
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
	line := trosces.keyboard.header.keyHeight + trosces.keyboard.trail.length.Beats()*trosces.keyboard.trail.beatSize
	path.MoveTo(0, line)
	path.LineTo(float32(x), line)
	path.LineTo(float32(x), line+256)
	path.LineTo(0, line+256)
	path.Fill(screen, &vector.FillOptions{Color: color.Black})
}

func (trosces *Trosces) Layout(outsideWidth, outsideHeight int) (int, int) {
	height := float32(outsideHeight) - trosces.keyboard.header.keyHeight
	trosces.keyboard.trail.SetBeatSize(height / 4)
	trosces.drums.trail.SetBeatSize(height / 4)
	trosces.layers.trail.SetBeatSize(height / 128)
	return outsideWidth, outsideHeight
}
