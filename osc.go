package main

import (
	"flag"
	"log"
	"time"

	"github.com/hypebeast/go-osc/osc"
)

var (
	oscAddr = flag.String("osc-addr", "127.0.0.1:8765", "UDP IP:port to listen for OSC messages")
)

func LaunchOSCServer(trosces *Trosces) {
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

		trosces.keyboardTrail.Span(instrument, int(note), duration)
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
}
