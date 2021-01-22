package main

import (
	"math"
	"time"
)

type Pulse struct {
	epoch time.Time
	bpm   float32
}

func NewPulse(bpm float32) *Pulse {
	return &Pulse{
		epoch: time.Now(),
		bpm:   bpm,
	}
}

func (p *Pulse) Now() Time {
	return Time{beat: float32(time.Now().Sub(p.epoch).Minutes()) * p.bpm}
}

func (p *Pulse) Sync(bpm float32) {
	// Current logical beat
	oldBeat := p.Now().Delta(Time{}).Beats()
	// We want the beat to occur exactly now
	wantBeat := float32(math.Round(float64(oldBeat)))
	// Adjust epoch such that with new BPM results in target number of beats
	p.epoch = time.Now().Add(-time.Duration((wantBeat / bpm) * float32(time.Minute)))
	p.bpm = bpm
}

type Duration struct {
	beats float32
}

func Beats(beats float32) Duration {
	return Duration{beats: beats}
}

func Forever() Duration {
	return Duration{beats: float32(math.Inf(1))}
}

func (d Duration) Beats() float32 {
	return d.beats
}

func (d Duration) IsZero() bool {
	return d.beats == 0
}

type Time struct {
	beat float32
}

func (b Time) After(other Time) bool {
	return b.beat > other.beat
}

func (b Time) Before(other Time) bool {
	return b.beat < other.beat
}

func (b Time) IsZero() bool {
	return b.beat == 0
}

func (b Time) Add(d Duration) Time {
	return Time{beat: b.beat + d.beats}
}

func (b Time) Sub(d Duration) Time {
	return Time{beat: b.beat - d.beats}
}

func (b Time) Delta(other Time) Duration {
	return Duration{beats: b.beat - other.beat}
}
func (b Time) Truncate(d Duration) Time {
	return Time{beat: float32(math.Trunc(float64(b.beat/d.beats))) * d.beats}
}
