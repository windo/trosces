package main

import (
	"errors"
	"flag"
	"log"
	"os"
	"runtime/pprof"
	"runtime/trace"

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
