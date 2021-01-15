package main

import (
	"errors"
	"flag"
	"image/color"
	"log"
	"math/rand"
	"os"
	"runtime/pprof"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hypebeast/go-osc/osc"
)

var (
	simulateInput = flag.Bool("simulate-input", false, "Simulate random input being received")
	cpuProfile    = flag.String("cpu-profile", "", "Path to CPU profile to be written")
)

type Game struct {
	keyboard *Keyboard
	track    *Track

	stop chan struct{}
}

type Finished struct{}

func (f Finished) Error() string { return "Game finished" }

func (g *Game) Update() error {
	log.Printf("FPS=%.1f TPS=%.1f", ebiten.CurrentFPS(), ebiten.CurrentTPS())
	if *simulateInput {
		if rand.Float32() < 0.05 {
			base, err := NewNote("c4")
			if err != nil {
				log.Fatalf("Could not parse note: %v", err)
			}
			note := base + Note(rand.Intn(12))
			g.track.Trace(
				rand.Intn(7),
				int(note),
				time.Duration(rand.Float32()*float32(time.Second)),
			)
		}
	}

	if int(g.keyboard.min) != g.track.minPos {
		g.keyboard.min = Note(g.track.minPos)
		g.keyboard.cached = nil
	}
	if int(g.keyboard.max) != g.track.maxPos {
		g.keyboard.max = Note(g.track.maxPos)
		g.keyboard.cached = nil
	}

	select {
	case <-g.stop:
		return Finished{}
	default:
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyQ) {
		return Finished{}
	}
	return nil
}

func (g *Game) Draw(screen *ebiten.Image) {
	screen.Fill(color.Black)
	g.keyboard.Draw(screen)
	g.track.Draw(screen)
}

func (g *Game) Layout(outsideWidth, outsideHeight int) (int, int) {
	return outsideWidth, outsideHeight
}

func main() {
	flag.Parse()

	if *cpuProfile != "" {
		f, err := os.Create(*cpuProfile)
		if err != nil {
			log.Fatal("Could not create CPU profile: ", err)
		}
		defer f.Close() // error handling omitted for example
		if err := pprof.StartCPUProfile(f); err != nil {
			log.Fatal("Could not start CPU profile: ", err)
		}
		defer pprof.StopCPUProfile()
	}

	g := &Game{
		keyboard: &Keyboard{
			min:         Note(40),
			max:         Note(50),
			keyWidth:    14,
			keyHeight:   20,
			borderWidth: 2,
			white:       color.RGBA{0xfd, 0xe5, 0xe5, 0xff},
			black:       color.RGBA{0x2e, 0x21, 0x21, 0xff},
		},
		track: NewTrack(64, 8*time.Second),
		stop:  make(chan struct{}),
	}
	ebiten.SetWindowTitle("TrOSCes")
	ebiten.SetWindowResizable(true)

	addr := "127.0.0.1:8765"
	d := osc.NewStandardDispatcher()
	d.AddMsgHandler("/play", func(msg *osc.Message) {
		if len(msg.Arguments) < 2 {
			log.Printf("Too few arguments for /play")
			return
		}
		if len(msg.Arguments) > 3 {
			log.Printf("Too many arguments for /play")
			return
		}
		instrument32, ok := msg.Arguments[0].(int32)
		if !ok {
			log.Printf("First /play argument not an integer")
			return
		}
		instrument := int(instrument32)
		noteStr, ok := msg.Arguments[1].(string)
		if !ok {
			log.Printf("Second /play argument not a string")
			return
		}
		note, err := NewNote(noteStr)
		if err != nil {
			log.Printf("Second /play argument not a note: %v", err)
			return
		}
		var d time.Duration
		if len(msg.Arguments) == 3 {
			f32, ok := msg.Arguments[2].(float32)
			if !ok {
				f64, ok := msg.Arguments[2].(float64)
				if !ok {
					log.Printf("Third /play argument not a float")
					return

				} else {
					d = time.Duration(f64 * float64(time.Second))
				}
			} else {
				d = time.Duration(f32 * float32(time.Second))
			}
		}
		g.track.Trace(instrument, int(note), d)
	})
	server := &osc.Server{
		Addr:       addr,
		Dispatcher: d,
	}

	go func() {
		if err := server.ListenAndServe(); err != nil {
			log.Fatalf("Failed to serve: %+v", err)
		}
	}()

	err := ebiten.RunGame(g)
	if err != nil && !errors.Is(err, Finished{}) {
		log.Fatal(err)
	}
}
