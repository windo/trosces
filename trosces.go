package main

import (
	"context"
	"errors"
	"log"
	"math/rand"
	"runtime/trace"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
)

type Trosces struct {
	keyboard      *Header
	keyboardTrail *Trail

	drums     *Header
	drumTrail *Trail

	layers     *Header
	layerTrail *Trail

	simulationTicker <-chan time.Time
}

func NewTrosces() *Trosces {
	log.Printf("Creating new Trosces")
	return &Trosces{
		keyboard:      NewHeader(14, 30),
		keyboardTrail: NewTrail(256, 4*time.Second),

		drums:     NewHeader(28, 30),
		drumTrail: NewTrail(256, 4*time.Second),

		layers:     NewHeader(28, 30),
		layerTrail: NewTrail(8, 120*time.Second),
	}
}

type Track struct {
	header Header
	trail  *Trail
}

var Finished = errors.New("Trosces finished")

func (trosces *Trosces) Update() error {
	_, task := trace.NewTask(context.Background(), "UpdateTrosces")
	defer task.End()

	// Maybe inject some synthetic events.
	if *simulateInput {
		if rand.Float32() < 0.1 {
			note := 32 + rand.Intn(4*12)
			trosces.keyboardTrail.Span(
				rand.Intn(7),
				note,
				time.Duration(rand.Float32()*float32(time.Second)),
			)
		}
		if trosces.simulationTicker == nil {
			trosces.simulationTicker = time.Tick(time.Second)
		}
		select {
		case <-trosces.simulationTicker:
			go func() {
				for i := 0; i < 2; i++ {
					trosces.drumTrail.Span(0, 0, time.Second/16)
					time.Sleep(time.Second / 2)
				}
			}()
			go func() {
				time.Sleep(time.Second / 4)
				trosces.drumTrail.Span(1, 1, time.Second/16)
				time.Sleep(time.Second / 2)
				trosces.drumTrail.Span(1, 1, time.Second/16)
				time.Sleep(time.Second / 4 / 4 * 3)
				trosces.drumTrail.Span(1, 1, time.Second/16)
			}()
			go func() {
				for i := 0; i < 8; i++ {
					trosces.drumTrail.Span(2, 2, time.Second/16)
					time.Sleep(time.Second / 8)
				}
			}()
		default:
		}
	}

	// Header matches the trail.
	trosces.keyboard.SetRange(trosces.keyboardTrail.minPos, trosces.keyboardTrail.maxPos)
	trosces.keyboard.SetActive(trosces.keyboardTrail.ActivePos())
	trosces.drums.SetRange(trosces.drumTrail.minPos, trosces.drumTrail.maxPos)
	trosces.drums.SetActive(trosces.drumTrail.ActivePos())
	trosces.layers.SetRange(trosces.layerTrail.minPos, trosces.layerTrail.maxPos)
	trosces.layers.SetActive(trosces.layerTrail.ActivePos())

	// Grid steps
	if inpututil.IsKeyJustPressed(ebiten.Key1) {
		trosces.keyboardTrail.SetGridSteps(11)
	}
	if inpututil.IsKeyJustPressed(ebiten.Key3) {
		trosces.keyboardTrail.SetGridSteps(3)
	}
	if inpututil.IsKeyJustPressed(ebiten.Key4) {
		trosces.keyboardTrail.SetGridSteps(4)
	}
	if inpututil.IsKeyJustPressed(ebiten.Key5) {
		trosces.keyboardTrail.SetGridSteps(5)
	}
	if inpututil.IsKeyJustPressed(ebiten.Key6) {
		trosces.keyboardTrail.SetGridSteps(6)
	}
	if inpututil.IsKeyJustPressed(ebiten.Key7) {
		trosces.keyboardTrail.SetGridSteps(7)
	}

	// Maybe finish.
	if inpututil.IsKeyJustPressed(ebiten.KeyQ) || inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
		return Finished
	}

	return nil
}

func (trosces *Trosces) Draw(screen *ebiten.Image) {
	ctxt, task := trace.NewTask(context.Background(), "DrawTrosces")
	defer task.End()

	var x float64
	trailOp := ebiten.DrawImageOptions{}
	trailOp.GeoM.Translate(x, float64(trosces.keyboard.keyHeight))
	trosces.keyboardTrail.Draw(ctxt, screen, &trailOp)
	headerOp := ebiten.DrawImageOptions{}
	headerOp.GeoM.Translate(x, 0)
	trosces.keyboard.Draw(screen, &headerOp)

	x += float64(trosces.keyboard.Width())
	trailOp = ebiten.DrawImageOptions{}
	trailOp.GeoM.Translate(x, float64(trosces.drums.keyHeight))
	trosces.drumTrail.Draw(ctxt, screen, &trailOp)
	headerOp = ebiten.DrawImageOptions{}
	headerOp.GeoM.Translate(x, 0)
	trosces.drums.Draw(screen, &headerOp)

	x += float64(trosces.drums.Width())
	trailOp = ebiten.DrawImageOptions{}
	trailOp.GeoM.Translate(x, float64(trosces.layers.keyHeight))
	trosces.layerTrail.Draw(ctxt, screen, &trailOp)
	headerOp = ebiten.DrawImageOptions{}
	headerOp.GeoM.Translate(x, 0)
	trosces.layers.Draw(screen, &headerOp)
}

func (trosces *Trosces) Layout(outsideWidth, outsideHeight int) (int, int) {
	return outsideWidth, outsideHeight
}
