package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"math/rand"
	"os"
	"runtime/pprof"
	"runtime/trace"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hypebeast/go-osc/osc"
)

var (
	simulateInput = flag.Bool("simulate-input", false, "Simulate random input being received")
	cpuProfile    = flag.String("cpu-profile", "", "Path to CPU profile to be written")
	memProfile    = flag.String("mem-profile", "", "Path to memory profile to be written")
	traceFile     = flag.String("trace-file", "", "Path to trace file to be written")
	oscAddr       = flag.String("osc-addr", "127.0.0.1:8765", "UDP IP:port to listen for OSC messages")
)

type Game struct {
	keyboard *Keyboard
	track    *Track
}

type Finished struct{}

func (f Finished) Error() string { return "Game finished" }

func (g *Game) Update() error {
	_, task := trace.NewTask(context.Background(), "UpdateGame")
	defer task.End()

	// Maybe inject some synthetic events.
	if *simulateInput {
		if rand.Float32() < 0.1 {
			base, err := NewNote("c4")
			if err != nil {
				log.Fatalf("Could not parse note: %v", err)
			}
			note := base + Note(rand.Intn(36))
			g.track.Span(
				rand.Intn(7),
				int(note),
				time.Duration(rand.Float32()*float32(time.Second)),
			)
		}
	}

	// Keyboard matches the track.
	g.keyboard.SetRange(Note(g.track.minPos), Note(g.track.maxPos))

	// Grid steps
	if inpututil.IsKeyJustPressed(ebiten.Key3) {
		g.track.SetGridSteps(3)
	}
	if inpututil.IsKeyJustPressed(ebiten.Key4) {
		g.track.SetGridSteps(4)
	}
	if inpututil.IsKeyJustPressed(ebiten.Key5) {
		g.track.SetGridSteps(5)
	}

	// Maybe finish.
	if inpututil.IsKeyJustPressed(ebiten.KeyQ) || inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
		return Finished{}
	}

	return nil
}

func (g *Game) Draw(screen *ebiten.Image) {
	ctxt, task := trace.NewTask(context.Background(), "DrawGame")
	defer task.End()

	//screen.Fill(color.Black)
	trackOp := ebiten.DrawImageOptions{}
	trackOp.GeoM.Translate(0, float64(g.keyboard.keyHeight))
	g.track.Draw(ctxt, screen, &trackOp)
	g.keyboard.Draw(screen, &ebiten.DrawImageOptions{})
}

func (g *Game) Layout(outsideWidth, outsideHeight int) (int, int) {
	return outsideWidth, outsideHeight
}

func main() {
	flag.Parse()

	if *cpuProfile != "" {
		f, err := os.Create(*cpuProfile)
		if err != nil {
			log.Fatal("Could not create CPU profile file: ", err)
		}
		defer f.Close()
		if err := pprof.StartCPUProfile(f); err != nil {
			log.Fatal("Could not start CPU profile: ", err)
		}
		defer pprof.StopCPUProfile()
	}
	if *traceFile != "" {
		f, err := os.Create(*traceFile)
		if err != nil {
			log.Fatal("Could not create trace file: ", err)
		}
		defer f.Close()
		if err := trace.Start(f); err != nil {
			log.Fatal("Could not start tracing: ", err)
		}
		defer trace.Stop()
	}

	g := &Game{
		keyboard: NewKeyboard(14, 20),
		track:    NewTrack(256, 4*time.Second),
	}

	d := osc.NewStandardDispatcher()

	d.AddMsgHandler("/play", func(msg *osc.Message) {
		if len(msg.Arguments) < 2 || len(msg.Arguments) > 3 {
			log.Printf("Expected 2 or 3 arguments for /play, got: %d", len(msg.Arguments))
			return
		}

		var (
			note       Note
			duration   time.Duration
			instrument int
		)

		if instrument32, ok := msg.Arguments[0].(int32); !ok {
			log.Printf("First /play argument not an integer")
			return
		} else {
			instrument = int(instrument32)
		}

		if noteStr, ok := msg.Arguments[1].(string); !ok {
			log.Printf("Second /play argument not a string")
			return
		} else {
			var err error
			if note, err = NewNote(noteStr); err != nil {
				log.Printf("Second /play argument not a note: %v", err)
				return
			}
		}

		if len(msg.Arguments) == 3 {
			if f32, ok := msg.Arguments[2].(float32); !ok {
				if f64, ok := msg.Arguments[2].(float64); !ok {
					log.Printf("Third /play argument not a float")
					return
				} else {
					duration = time.Duration(f64 * float64(time.Second))
				}
			} else {
				duration = time.Duration(f32 * float32(time.Second))
			}
		}

		g.track.Span(instrument, int(note), duration)
	})
	server := &osc.Server{
		Addr:       *oscAddr,
		Dispatcher: d,
	}

	go func() {
		if err := server.ListenAndServe(); err != nil {
			log.Fatalf("Failed to serve: %+v", err)
		}
	}()

	ebiten.SetWindowTitle("TrOSCes")
	ebiten.SetWindowResizable(true)
	//ebiten.SetScreenClearedEveryFrame(false)
	//ebiten.SetMaxTPS(60)

	err := ebiten.RunGame(g)
	if err != nil && !errors.Is(err, Finished{}) {
		log.Fatal(err)
	}

	if *memProfile != "" {
		f, err := os.Create(*memProfile)
		if err != nil {
			log.Fatal("Could not create memory profile file: ", err)
		}
		defer f.Close()
		if err := pprof.WriteHeapProfile(f); err != nil {
			log.Fatal("Could not write memory profile: ", err)
		}
	}
}
