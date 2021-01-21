package main

import (
	"errors"
	"flag"
	"log"
	"math/rand"
	"os"
	"runtime/pprof"
	"runtime/trace"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
)

var (
	simulateInput = flag.Bool("simulate-input", false, "Simulate random input being received")
	cpuProfile    = flag.String("cpu-profile", "", "Path to CPU profile to be written")
	memProfile    = flag.String("mem-profile", "", "Path to memory profile to be written")
	traceFile     = flag.String("trace-file", "", "Path to trace file to be written")
)

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

	trosces := NewTrosces()

	// Maybe inject some synthetic events.
	if *simulateInput {
		// Random
		go func() {
			for {
				if rand.Float32() < 0.1 {
					note := 32 + rand.Intn(4*12)
					trosces.keyboard.trail.Span(
						rand.Intn(7),
						note,
						Beats(rand.Float32()*float32(time.Second)),
					)
				}
				time.Sleep(time.Second / 60)
			}
		}()

		// Synchronized
		go func() {
			simulationTicker := time.Tick(4 * time.Second)
			for {
				// Drums
				go func() {
					for i := 0; i < 4; i++ {
						go func() {
							for i := 0; i < 2; i++ {
								trosces.drums.trail.Span(0, 0, Beats(1.0/16))
								time.Sleep(time.Second / 2)
							}
						}()
						go func() {
							time.Sleep(time.Second / 4)
							trosces.drums.trail.Span(1, 1, Beats(1.0/16))
							time.Sleep(time.Second / 2)
							trosces.drums.trail.Span(1, 1, Beats(1.0/16))
							time.Sleep(time.Second / 4 / 4 * 3)
							trosces.drums.trail.Span(1, 1, Beats(1.0/16))
						}()
						go func() {
							for i := 0; i < 8; i++ {
								trosces.drums.trail.Span(2, 2, Beats(1.0/16))
								time.Sleep(time.Second / 8)
							}
						}()
						time.Sleep(time.Second)
					}
				}()

				// Layers
				go func() {
					if rand.Intn(4) == 0 {
						trosces.layers.trail.Span(1, 0, Beats(4))
					} else {
						trosces.layers.trail.Span(0, 0, Beats(4))
					}
					if rand.Intn(4) == 0 {
						trosces.layers.trail.Span(0, 1, Beats(4))
					}
				}()

				<-simulationTicker
			}
		}()
	}
	LaunchOSCServer(trosces)

	ebiten.SetWindowTitle("TrOSCes")
	ebiten.SetWindowResizable(true)
	//ebiten.SetScreenClearedEveryFrame(false)
	//ebiten.SetMaxTPS(60)

	err := ebiten.RunGame(trosces)
	if err != nil && !errors.Is(err, Finished) {
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
