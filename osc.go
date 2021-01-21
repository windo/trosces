package main

import (
	"flag"
	"fmt"
	"log"

	"github.com/hypebeast/go-osc/osc"
)

var (
	oscAddr = flag.String("osc-addr", "127.0.0.1:8765", "UDP IP:port to listen for OSC messages")
)

func CheckArgs(args []interface{}, min int, max int) error {
	if len(args) < min || len(args) > max {
		return fmt.Errorf("expected %d to %d arguments, got: %d", min, max, len(args))
	}
	return nil
}

func NameArg(arg interface{}) (string, error) {
	if name, ok := arg.(string); ok {
		return name, nil
	} else {
		return "", fmt.Errorf("not a string")
	}
}

func NoteArg(arg interface{}) (Note, error) {
	if name, err := NameArg(arg); err != nil {
		return 0, err
	} else {
		if note, err := NewNote(name); err != nil {
			return 0, err
		} else {
			return note, nil
		}
	}
}

func NumberArg(arg interface{}) (int, error) {
	if i32, ok := arg.(int32); !ok {
		return 0, fmt.Errorf("not an integer")
	} else {
		return int(i32), nil
	}
}

func DurationArg(arg interface{}) (Duration, error) {
	if f32, ok := arg.(float32); !ok {
		if f64, ok := arg.(float64); !ok {
			if i, err := NumberArg(arg); err != nil {
				return Beats(0), fmt.Errorf("not a number")
			} else {
				return Beats(float32(i)), nil
			}
		} else {
			return Beats(float32(f64)), nil
		}
	} else {
		return Beats(f32), nil
	}
}

func LaunchOSCServer(trosces *Trosces) {
	d := osc.NewStandardDispatcher()

	d.AddMsgHandler("/play", func(msg *osc.Message) {
		var err error
		if err = CheckArgs(msg.Arguments, 2, 3); err != nil {
			log.Printf("Invalid /play: %v", err)
			return
		}

		var (
			instrument string
			note       Note
			duration   Duration
		)

		if instrument, err = NameArg(msg.Arguments[0]); err != nil {
			log.Printf("Invalid /play[0] instrument: %v", err)
			return
		}

		if note, err = NoteArg(msg.Arguments[1]); err != nil {
			log.Printf("Invalid /play[1] note: %v", err)
			return
		}

		if len(msg.Arguments) == 3 {
			if duration, err = DurationArg(msg.Arguments[2]); err != nil {
				log.Printf("Invalid /play[2] duration: %v", err)
				return
			}
		}

		trosces.PlayNote(instrument, note, duration)
	})

	d.AddMsgHandler("/drum", func(msg *osc.Message) {
		var err error
		if err = CheckArgs(msg.Arguments, 1, 2); err != nil {
			log.Printf("Invalid /drum: %v", err)
			return
		}

		var (
			instrument string
			duration   Duration
		)

		if instrument, err = NameArg(msg.Arguments[0]); err != nil {
			log.Printf("Invalid /drum[0] instrument: %v", err)
			return
		}

		if len(msg.Arguments) == 2 {
			if duration, err = DurationArg(msg.Arguments[1]); err != nil {
				log.Printf("Invalid /drum[1] duration: %v", err)
				return
			}
		}

		trosces.PlayDrum(instrument, duration)
	})

	d.AddMsgHandler("/layer", func(msg *osc.Message) {
		var err error
		if err = CheckArgs(msg.Arguments, 2, 3); err != nil {
			log.Printf("Invalid /layer: %v", err)
			return
		}

		var (
			name     string
			duration Duration
			variant  string
		)

		if name, err = NameArg(msg.Arguments[0]); err != nil {
			log.Printf("Invalid /layer[0] name: %v", err)
			return
		}

		if duration, err = DurationArg(msg.Arguments[1]); err != nil {
			log.Printf("Invalid /layer[1] duration: %v", err)
			return
		}

		if len(msg.Arguments) == 3 {
			if variant, err = NameArg(msg.Arguments[2]); err != nil {
				log.Printf("Invalid /layer[2] variant: %v", err)
				return
			}
		}

		trosces.PlayLayer(name, duration, variant)
	})

	d.AddMsgHandler("/sync", func(msg *osc.Message) {
		var err error
		if err = CheckArgs(msg.Arguments, 1, 1); err != nil {
			log.Printf("Invalid /sync: %v", err)
			return
		}

		var bpm int

		if bpm, err = NumberArg(msg.Arguments[0]); err != nil {
			log.Printf("Invalid /sync[0] bpm: %v", err)
			return
		}

		trosces.Sync(bpm)
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
