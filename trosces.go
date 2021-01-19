package main

import (
	"context"
	"errors"
	"log"
	"runtime/trace"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
)

type Trosces struct {
	keyboard *Track
	drums    *Track
	layers   *Track
}

func NewTrosces() *Trosces {
	log.Printf("Creating new Trosces")
	trosces := &Trosces{
		keyboard: &Track{
			header: NewHeader(15, 30),
			trail:  NewTrail(time.Second, 4*time.Second, 256, 15),
		},
		drums: &Track{
			header: NewHeader(30, 30),
			trail:  NewTrail(time.Second, 4*time.Second, 256, 30),
		},
		layers: &Track{
			header: NewHeader(30, 30),
			trail:  NewTrail(8*time.Second, 120*time.Second, 8, 30),
		},
	}
	trosces.keyboard.header.keyboard = true
	trosces.drums.header.borderWidth = 2
	trosces.drums.trail.borderWidth = 2
	trosces.layers.header.borderWidth = 2
	trosces.layers.trail.borderWidth = 2

	return trosces
}

type Track struct {
	header *Header
	trail  *Trail
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
}

func (trosces *Trosces) Layout(outsideWidth, outsideHeight int) (int, int) {
	return outsideWidth, outsideHeight
}
