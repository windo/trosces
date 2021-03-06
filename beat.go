package main

import (
	"fmt"
	"math"
	"time"
)

type Pulse struct {
	epoch  time.Time
	frozen Time
	bpm    float32
}

func NewPulse(bpm float32) *Pulse {
	return &Pulse{
		epoch: time.Now(),
		bpm:   bpm,
	}
}

// Current beat time.
func (p *Pulse) Now() Time {
	return Time{beat: float32(time.Now().Sub(p.epoch).Minutes()) * p.bpm}
}

func (p *Pulse) ToggleFrozen() {
	if p.frozen.IsZero() {
		p.frozen = p.Now()
	} else {
		p.frozen = Time{}
	}
}

// Current (potentially frozen) time.
func (p *Pulse) Horizon() Time {
	if p.frozen.IsZero() {
		return p.Now()
	} else {
		return p.frozen
	}
}

// Update BPM and adjust epoch to the beat happening right this instant.
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

func (d Duration) String() string {
	return fmt.Sprintf("beats=%.2f", d.beats)
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

func (d Duration) VisuallyZero() bool {
	return math.Abs(float64(d.beats)) < float64(VisualSlack.beats)
}

type Time struct {
	beat float32
}

func (b Time) String() string {
	return fmt.Sprintf("beat=%.2f", b.beat)
}

func OnBeat(beat float32) Time {
	return Time{beat: beat}
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

func (b Time) VisuallyClose(other Time) bool {
	return other.Delta(b).VisuallyZero()
}

func (b Time) Same(other Time) bool {
	return other.Delta(b).IsZero()
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
