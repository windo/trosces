package main

import (
	"math"
	"testing"
)

func AlmostEqual(a, b float32) bool {
	return math.Abs(float64(b-a)) < 1e-3
}

func TestSubSpanBounds(t *testing.T) {
	trail := &Trail{
		beatSize:    64,
		bucketSize:  Beats(1),
		borderWidth: 2,
		posWidth:    14,
		minPos:      0,
	}

	for _, tc := range []struct {
		name                                          string
		subspan                                       *SubSpan
		wantStart, wantEnd, wantOffset, wantEndOffset float32
	}{
		{
			name: "single full-length span",
			subspan: &SubSpan{
				span: &Span{
					pos:   0,
					start: OnBeat(1),
					end:   OnBeat(2),
				},
				start:      OnBeat(1),
				end:        OnBeat(2),
				subindex:   0,
				subindices: 1,
				first:      true,
				last:       true,
			},
			// Borders all around
			wantStart:     62,
			wantEnd:       2,
			wantOffset:    2,
			wantEndOffset: 12,
		},
		{
			name: "single full-length mid-span",
			subspan: &SubSpan{
				span: &Span{
					pos:   0,
					start: OnBeat(1),
					end:   OnBeat(2),
				},
				start:      OnBeat(1),
				end:        OnBeat(2),
				subindex:   0,
				subindices: 1,
				first:      false,
				last:       false,
			},
			// No borders
			wantStart: 64,
			wantEnd:   0,
			// Borders
			wantOffset:    2,
			wantEndOffset: 12,
		},
		{
			name: "single tiny span",
			subspan: &SubSpan{
				span: &Span{
					pos:   0,
					start: OnBeat(1.5),
					end:   OnBeat(1.501),
				},
				start:      OnBeat(1.5),
				end:        OnBeat(1.501),
				subindex:   0,
				subindices: 1,
				first:      true,
				last:       true,
			},
			// Always at least 1 pixel
			wantStart: 32,
			wantEnd:   31,
			// Borders
			wantOffset:    2,
			wantEndOffset: 12,
		},
		{
			name: "single oversized full span",
			subspan: &SubSpan{
				span: &Span{
					pos:   0,
					start: OnBeat(0.5),
					end:   OnBeat(2.5),
				},
				start:      OnBeat(0.5),
				end:        OnBeat(2.5),
				subindex:   0,
				subindices: 1,
				first:      true,
				last:       true,
			},
			// No borders - they are out of bounds
			wantStart: 64,
			wantEnd:   0,
			// Borders
			wantOffset:    2,
			wantEndOffset: 12,
		},
		{
			name: "two overlapping left",
			subspan: &SubSpan{
				span: &Span{
					pos:   0,
					start: OnBeat(1),
					end:   OnBeat(2),
				},
				start:      OnBeat(1),
				end:        OnBeat(2),
				subindex:   0,
				subindices: 2,
				first:      false,
				last:       false,
			},
			wantStart: 64,
			wantEnd:   0,
			// Left border, right shared border (half-width)
			wantOffset:    2,
			wantEndOffset: 6,
		},
		{
			name: "two overlapping right",
			subspan: &SubSpan{
				span: &Span{
					pos:   0,
					start: OnBeat(1),
					end:   OnBeat(2),
				},
				start:      OnBeat(1),
				end:        OnBeat(2),
				subindex:   1,
				subindices: 2,
				first:      false,
				last:       false,
			},
			wantStart: 64,
			wantEnd:   0,
			// Right border, left shared border (half-width)
			wantOffset:    8,
			wantEndOffset: 12,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			start, end, offset, endOffset := trail.subSpanBounds(OnBeat(1), tc.subspan)
			if !AlmostEqual(start, tc.wantStart) {
				t.Errorf("want start: %.3f, got: %.3f", tc.wantStart, start)
			}
			if !AlmostEqual(end, tc.wantEnd) {
				t.Errorf("want end: %.3f, got: %.3f", tc.wantEnd, end)
			}
			if !AlmostEqual(offset, tc.wantOffset) {
				t.Errorf("want offset: %.3f, got: %.3f", tc.wantOffset, offset)
			}
			if !AlmostEqual(endOffset, tc.wantEndOffset) {
				t.Errorf("want endOffset: %.3f, got: %.3f", tc.wantEndOffset, endOffset)
			}
		})
	}
}

func TestSubindex(t *testing.T) {
	for _, tc := range []struct {
		name         string
		spans        []*Span
		wantSubSpans []*SubSpan
	}{
		{
			name: "no overlap in time",
			spans: []*Span{
				&Span{
					id:    0,
					pos:   0,
					start: OnBeat(0),
					end:   OnBeat(0.5),
				},
				&Span{
					id:    1,
					pos:   0,
					start: OnBeat(0.5),
					end:   OnBeat(1.0),
				},
			},
			wantSubSpans: []*SubSpan{
				&SubSpan{
					span:       &Span{id: 0},
					start:      OnBeat(0),
					end:        OnBeat(0.5),
					subindex:   0,
					subindices: 1,
					first:      true,
					last:       true,
				},
				&SubSpan{
					span:       &Span{id: 1},
					start:      OnBeat(0.5),
					end:        OnBeat(1.0),
					subindex:   0,
					subindices: 1,
					first:      true,
					last:       true,
				},
			},
		},
		{
			name: "no overlap in pos",
			spans: []*Span{
				&Span{
					id:    0,
					pos:   0,
					start: OnBeat(0),
					end:   OnBeat(1.0),
				},
				&Span{
					id:    1,
					pos:   1,
					start: OnBeat(0),
					end:   OnBeat(1.0),
				},
			},
			wantSubSpans: []*SubSpan{
				&SubSpan{
					span:       &Span{id: 0},
					start:      OnBeat(0),
					end:        OnBeat(1.0),
					subindex:   0,
					subindices: 1,
					first:      true,
					last:       true,
				},
				&SubSpan{
					span:       &Span{id: 1},
					start:      OnBeat(0),
					end:        OnBeat(1.0),
					subindex:   0,
					subindices: 1,
					first:      true,
					last:       true,
				},
			},
		},
		{
			name: "full overlap",
			spans: []*Span{
				&Span{
					id:    0,
					pos:   0,
					start: OnBeat(0),
					end:   OnBeat(1.0),
				},
				&Span{
					id:    1,
					pos:   0,
					start: OnBeat(0),
					end:   OnBeat(1.0),
				},
			},
			wantSubSpans: []*SubSpan{
				&SubSpan{
					span:       &Span{id: 0},
					start:      OnBeat(0),
					end:        OnBeat(1.0),
					subindex:   0,
					subindices: 2,
					first:      true,
					last:       true,
				},
				&SubSpan{
					span:       &Span{id: 1},
					start:      OnBeat(0),
					end:        OnBeat(1.0),
					subindex:   1,
					subindices: 2,
					first:      true,
					last:       true,
				},
			},
		},
		{
			name: "partial overlap",
			spans: []*Span{
				&Span{
					id:    0,
					pos:   0,
					start: OnBeat(0),
					end:   OnBeat(0.8),
				},
				&Span{
					id:    1,
					pos:   0,
					start: OnBeat(0.2),
					end:   OnBeat(1.0),
				},
			},
			wantSubSpans: []*SubSpan{
				&SubSpan{
					span:       &Span{id: 0},
					start:      OnBeat(0),
					end:        OnBeat(0.2),
					subindex:   0,
					subindices: 1,
					first:      true,
					last:       false,
				},
				&SubSpan{
					span:       &Span{id: 1},
					start:      OnBeat(0.2),
					end:        OnBeat(0.8),
					subindex:   1,
					subindices: 2,
					first:      true,
					last:       false,
				},
				&SubSpan{
					span:       &Span{id: 0},
					start:      OnBeat(0.2),
					end:        OnBeat(0.8),
					subindex:   0,
					subindices: 2,
					first:      false,
					last:       true,
				},
				&SubSpan{
					span:       &Span{id: 1},
					start:      OnBeat(0.8),
					end:        OnBeat(1.0),
					subindex:   0,
					subindices: 1,
					first:      false,
					last:       true,
				},
			},
		},
		{
			name: "wholly contained",
			spans: []*Span{
				&Span{
					id:    0,
					pos:   0,
					start: OnBeat(0),
					end:   OnBeat(1.0),
				},
				&Span{
					id:    1,
					pos:   0,
					start: OnBeat(0.2),
					end:   OnBeat(0.8),
				},
			},
			wantSubSpans: []*SubSpan{
				&SubSpan{
					span:       &Span{id: 0},
					start:      OnBeat(0),
					end:        OnBeat(0.2),
					subindex:   0,
					subindices: 1,
					first:      true,
					last:       false,
				},
				&SubSpan{
					span:       &Span{id: 1},
					start:      OnBeat(0.2),
					end:        OnBeat(0.8),
					subindex:   1,
					subindices: 2,
					first:      true,
					last:       true,
				},
				&SubSpan{
					span:       &Span{id: 0},
					start:      OnBeat(0.2),
					end:        OnBeat(0.8),
					subindex:   0,
					subindices: 2,
					first:      false,
					last:       false,
				},
				&SubSpan{
					span:       &Span{id: 0},
					start:      OnBeat(0.8),
					end:        OnBeat(1.0),
					subindex:   0,
					subindices: 1,
					first:      false,
					last:       true,
				},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			subSpans := Subindex(tc.spans)
			for i := 0; i < len(subSpans) && i < len(tc.wantSubSpans); i++ {
				if i >= len(tc.wantSubSpans) {
					t.Errorf("extra subspan[%d] = %v", i, subSpans[i])
					continue
				}
				if i >= len(subSpans) {
					t.Errorf("missing subspan[%d] = %v", i, tc.wantSubSpans[i])
					continue
				}
				got := subSpans[i]
				want := tc.wantSubSpans[i]
				if !AlmostEqual(got.start.beat, want.start.beat) || !AlmostEqual(got.end.beat, want.end.beat) {
					t.Errorf("subspan[%d] want start->end: %.3f->%.3f, got: %.3f->%.3f", i, want.start, want.end, got.start, got.end)
				}
				if got.subindex != want.subindex || got.subindices != want.subindices {
					t.Errorf("subspan[%d] want indices: %d/%d, got: %d/%d", i, want.subindex, want.subindices, got.subindex, got.subindices)
				}
				if got.first != want.first {
					t.Errorf("subspan[%d] want first: %t, got: %t", i, want.first, got.first)
				}
				if got.last != want.last {
					t.Errorf("subspan[%d] want last: %t, got: %t", i, want.last, got.last)
				}
				if got.span.id != want.span.id {
					t.Errorf("subspan[%d] want span id: %d, got: %d", i, want.span.id, got.span.id)
				}
			}
		})
	}
}
