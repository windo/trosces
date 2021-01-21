package main

import (
	"context"
	"fmt"
	"image/color"
	"log"
	"math"
	"runtime/trace"
	"sort"
	"sync"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

var VisualSlack Duration = Duration{beats: 0.01}

type Span struct {
	// "Instrument" or other category ID (for the same position)
	id int
	// Position/lane on the timeline
	pos int

	// Start of the span.
	start Time
	// End - potentially ~far in the future.
	end Time
}

func (span *Span) InRange(start Time, end Time) bool {
	if span.end.Before(start) {
		return false
	}
	if span.start.After(end) {
		return false
	}
	return true
}

func (span *Span) InVisualRange(start Time, end Time) bool {
	if span.end.Before(start.Add(VisualSlack)) {
		return false
	}
	if span.start.After(end.Sub(VisualSlack)) {
		return false
	}
	return true
}

func (span *Span) String() string {
	return fmt.Sprintf("%d@%d [%.2f:%.2f]", span.id, span.pos, span.start, span.end)
}

var (
	// http://personal.sron.nl/~pault/ "bright"
	spanPalette = []color.Color{
		color.RGBA{0x44, 0x77, 0xaa, 0xff}, // Blue
		color.RGBA{0x22, 0x88, 0x33, 0xff}, // Green
		color.RGBA{0xcc, 0xbb, 0x44, 0xff}, // Yellow
		color.RGBA{0xee, 0x66, 0x77, 0xff}, // Red
		color.RGBA{0x66, 0xcc, 0xee, 0xff}, // Cyan (was second)
		color.RGBA{0xaa, 0x33, 0x77, 0xff}, // Purple
		color.RGBA{0xbb, 0xbb, 0xbb, 0xff}, // Grey
	}
)

type SpanBucket struct {
	start Time
	end   Time
	spans []*Span
}

func (bucket *SpanBucket) InRange(start Time, end Time) bool {
	if bucket.end.Before(start) {
		return false
	}
	if bucket.start.After(end) {
		return false
	}
	return true
}

func (bucket *SpanBucket) UpdateEnd() {
	var latest Time
	for _, span := range bucket.spans {
		if span.end.After(latest) {
			latest = span.end
		}
	}
	bucket.end = latest
}

func (bucket *SpanBucket) Validate() error {
	for _, span := range bucket.spans {
		if span.start.Before(bucket.start) || span.start.After(bucket.end) || span.end.Before(bucket.start) {
			return fmt.Errorf("Out of bucket %v span: %v!", bucket, span)
		}
	}
	return nil
}

type Trail struct {
	// Spans bucketed by periods of duration `bucketSize`
	buckets map[Time]*SpanBucket

	minPos int
	maxPos int
	active []*Span

	// Images of spans from [time : time + bucketSize]
	cached      map[Time]*ebiten.Image
	cachedReady map[Time]bool
	grid        *ebiten.Image
	gridReady   bool
	unused      []*ebiten.Image

	secondSize float32
	bpm        float32
	gridSteps  int
	length     Duration
	bucketSize Duration

	posWidth    float32
	borderWidth float32

	pulse *Pulse

	mu sync.Mutex
}

func NewTrail(bucketSize Duration, length Duration, secondSize float32, posWidth float32) *Trail {
	log.Printf("New trail")
	trail := Trail{
		buckets: map[Time]*SpanBucket{},

		cached:      map[Time]*ebiten.Image{},
		cachedReady: map[Time]bool{},
		unused:      []*ebiten.Image{},

		minPos: 0,
		maxPos: 0,

		secondSize:  secondSize,
		bucketSize:  bucketSize,
		length:      length,
		borderWidth: 1,
		posWidth:    posWidth,
		gridSteps:   4,
	}

	go func() {
		ticker := time.NewTicker(5 * time.Second)
		for {
			<-ticker.C
			trail.cleanup()
		}
	}()

	return &trail
}

func (trail *Trail) Span(id int, pos int, d Duration) {
	defer trace.StartRegion(context.Background(), "NewSpan").End()
	now := trail.pulse.Now()
	bucketTime := now.Truncate(trail.bucketSize)

	if id >= len(spanPalette) {
		log.Printf("Instrument ID %d too big, wrapping around", id)
		id %= len(spanPalette)
	}

	trail.mu.Lock()
	defer trail.mu.Unlock()

	if len(trail.buckets) == 0 {
		trail.minPos = pos
		trail.maxPos = pos
		trail.resetAll()
	} else if pos < trail.minPos {
		trail.minPos = pos
		trail.resetAll()
	} else if pos > trail.maxPos {
		trail.maxPos = pos
		trail.resetAll()
	}

	span := &Span{
		id:    id,
		pos:   pos,
		start: now,
		end:   now.Add(d),
	}
	//log.Printf("New span: %s", span.String())

	var bucket *SpanBucket
	if trail.buckets[bucketTime] == nil {
		bucket = &SpanBucket{
			start: bucketTime,
			end:   span.end,
		}
		trail.buckets[bucketTime] = bucket
	} else {
		bucket = trail.buckets[bucketTime]
		if span.end.After(bucket.end) {
			bucket.end = span.end
		}
	}

	// Invalidate cached bucket image
	trail.redrawBucket(bucketTime)
	bucket.spans = append(bucket.spans, span)
	trail.active = append(trail.active, span)
}

// TODO: Unused as of now
func (trail *Trail) Stop(id int, pos int) {
	defer trace.StartRegion(context.Background(), "StopSpan").End()
	now := trail.pulse.Now()
	trail.mu.Lock()
	defer trail.mu.Unlock()

	for _, bucket := range trail.buckets {
		if bucket.end.Before(now) {
			continue
		}
		for _, span := range bucket.spans {
			if span.id == id && span.pos == pos && span.end.After(now) {
				span.end = now
				trail.redrawBucket(now.Truncate(trail.bucketSize))
				defer bucket.UpdateEnd()
				return
			}
		}
	}
}

func (trail *Trail) SetGridSteps(steps int) {
	trail.mu.Lock()
	defer trail.mu.Unlock()

	trail.gridSteps = steps
	trail.redrawAll()
}

func (trail *Trail) ActivePos() []int {
	trail.mu.Lock()
	defer trail.mu.Unlock()

	now := trail.pulse.Now()
	activeMap := map[int]struct{}{}
	active := []int{}

	i := 0
	for i < len(trail.active) {
		span := trail.active[i]
		if !span.InRange(now, now) {
			// TODO: Only place where cleanup of trail.active happens!
			trail.active = append(trail.active[:i], trail.active[i+1:]...)
			continue
		}
		if _, ok := activeMap[span.pos]; !ok {
			activeMap[span.pos] = struct{}{}
			active = append(active, span.pos)
		}
		i++
	}
	sort.Ints(active)
	return active
}

// Draw all the trail components.
func (trail *Trail) Draw(ctxt context.Context, image *ebiten.Image, op *ebiten.DrawImageOptions) {
	defer trace.StartRegion(ctxt, "DrawTrail").End()
	now := trail.pulse.Now()

	// History (time < now) flows away from 0.

	// Bucket covering now, extensing at most bucketSize to the future
	bucketTime := now.Truncate(trail.bucketSize)
	// End of the scroll trail
	trailEnd := now.Sub(trail.length)

	// Until we find a bucket that covers the end of the trail
	for bucketTime.After(trailEnd) {
		bucketOp := ebiten.DrawImageOptions{}
		bucketOp.GeoM = op.GeoM
		// bucket images contain [bucketTime+bucketSize (fresher edge, y=0) ... bucketTime (older edge, y>0)]
		// now -> on screen y=0, future -> on screen y<0
		offset := now.Delta(bucketTime.Add(trail.bucketSize)).Beats() * trail.secondSize
		bucketOp.GeoM.Translate(0, float64(offset))
		image.DrawImage(trail.getCached(ctxt, bucketTime), &bucketOp)
		// move to one older bucket
		bucketTime = bucketTime.Sub(trail.bucketSize)
	}
}

// Internal

func (trail *Trail) redrawBucket(bucketTime Time) {
	trail.cachedReady[bucketTime] = false
}

func (trail *Trail) redrawAll() {
	trail.cachedReady = map[Time]bool{}
	trail.gridReady = false
}

func (trail *Trail) resetAll() {
	// mu must be held

	disposeLater := func(image *ebiten.Image) {
		go func() {
			time.Sleep(5 * time.Second)
			image.Dispose()
		}()
	}

	for _, image := range trail.cached {
		disposeLater(image)
	}
	trail.cached = map[Time]*ebiten.Image{}

	if trail.grid != nil {
		disposeLater(trail.grid)
	}
	trail.grid = nil

	for _, image := range trail.unused {
		disposeLater(image)
	}
	trail.unused = []*ebiten.Image{}

	trail.redrawAll()
}

func (trail *Trail) allocateImage() *ebiten.Image {
	// mu must be held

	if len(trail.unused) > 0 {
		log.Printf("Reusing unused (out of %d)", len(trail.unused))
		image := trail.unused[len(trail.unused)-1]
		trail.unused = trail.unused[:len(trail.unused)-1]
		image.Clear()
		return image
	}

	log.Printf("Creating new image")
	return ebiten.NewImage(
		int(trail.posWidth*float32(trail.maxPos-trail.minPos+1)),
		int(trail.bucketSize.Beats()*trail.secondSize),
	)
}

// Discard old spans and cached images
func (trail *Trail) cleanup() {
	_, task := trace.NewTask(context.Background(), "cleanup")
	defer task.End()
	log.Printf("Starting cleanup")

	now := trail.pulse.Now()

	trail.mu.Lock()
	defer trail.mu.Unlock()

	// Cleanup old span buckets
	removeBuckets := []Time{}
	for bucketTime, bucket := range trail.buckets {
		if bucket.end.Before(now.Sub(trail.length)) {
			removeBuckets = append(removeBuckets, bucketTime)
		}
	}
	for _, bucketTime := range removeBuckets {
		delete(trail.buckets, bucketTime)
	}

	// Reuse images
	freeCached := []Time{}
	for bucketTime := range trail.cached {
		if bucketTime.Add(trail.bucketSize).Before(now.Sub(trail.length)) {
			freeCached = append(freeCached, bucketTime)
		}
	}
	for _, bucketTime := range freeCached {
		log.Printf("Adding %v to unused", bucketTime)
		image := trail.cached[bucketTime]
		trail.unused = append(trail.unused, image)
		delete(trail.cached, bucketTime)
	}

	log.Printf("Finished cleanup")
}

// Produce a (cached) grid for trail background.
func (trail *Trail) getGrid(ctxt context.Context) *ebiten.Image {
	defer trace.StartRegion(ctxt, "getCachedGrid").End()
	// mu must be taken
	if !trail.gridReady {
		if trail.grid == nil {
			trail.grid = trail.allocateImage()
		}

		// Key columns
		for pos := trail.minPos; pos <= trail.maxPos; pos++ {
			basePos := pos - trail.minPos

			path := vector.Path{}
			path.MoveTo(float32(basePos)*trail.posWidth, 0)
			path.LineTo(float32(basePos)*trail.posWidth, float32(trail.grid.Bounds().Max.Y))
			path.LineTo(float32(basePos+1)*trail.posWidth, float32(trail.grid.Bounds().Max.Y))
			path.LineTo(float32(basePos+1)*trail.posWidth, 0)

			var op vector.FillOptions
			switch {
			case pos%12 == 0:
				op.Color = color.RGBA{0x30, 0x30, 0x30, 0xff}
			case Note(pos).IsWhite():
				op.Color = color.RGBA{0x20, 0x20, 0x20, 0xff}
			default:
				op.Color = color.RGBA{0x10, 0x10, 0x10, 0xff}
			}

			path.Fill(trail.grid, &op)
		}

		// Timeline
		for t := float32(0); t < trail.bucketSize.Beats(); t += trail.bucketSize.Beats() / float32(trail.gridSteps) {
			baseTime := t * trail.secondSize

			path := vector.Path{}
			path.MoveTo(0, baseTime)
			path.LineTo(float32(trail.grid.Bounds().Max.X), baseTime)
			path.LineTo(float32(trail.grid.Bounds().Max.X), baseTime+trail.borderWidth)
			path.LineTo(0, baseTime+trail.borderWidth)

			var op vector.FillOptions
			switch {
			case baseTime == 0:
				op.Color = color.RGBA{0xc0, 0xc0, 0xc0, 0xff}
			default:
				op.Color = color.RGBA{0x80, 0x80, 0x80, 0xff}
			}

			path.Fill(trail.grid, &op)
		}

		// Updated!
		trail.gridReady = true
	}
	return trail.grid
}

func (trail *Trail) drawSubSpan(image *ebiten.Image, bucketTime Time, subSpan *SubSpan) {
	bucketEndTime := bucketTime.Add(trail.bucketSize)

	// X: note index
	baseOffset := float32(subSpan.span.pos-trail.minPos) * trail.posWidth
	subWidth := (trail.posWidth - 2*trail.borderWidth) / float32(subSpan.subindices)
	offset := baseOffset + trail.borderWidth + float32(subSpan.subindex)*subWidth

	// start==bucketEndTime -> y=0, older (start < bucketEndTime) -> y>0
	// start < bucketEndTime, no limit vs bucketTime
	start := float32(math.Min(
		float64(bucketEndTime.Delta(subSpan.start).Beats())*float64(trail.secondSize),
		float64(image.Bounds().Max.Y),
	))
	// end > bucketTime, no limit vs bucketEndTime
	end := float32(math.Max(float64(bucketEndTime.Delta(subSpan.end).Beats())*float64(trail.secondSize), 0))

	//log.Printf("Drawing: %v -> [%.1f : %.1f] in %v", span, start, end, imageBucketTime)

	if start < 0 {
		log.Printf("%v: start should be within bucket @%.1f: %.1f", subSpan, bucketTime, start)
		return
	}
	if end > float32(image.Bounds().Max.Y) {
		log.Printf("%v: end should be within bucket @%.1f: %.1f", subSpan, bucketTime, end)
		return
	}
	if end > start {
		log.Printf("%v: wrong order: %.1f > %.1f", subSpan, end, start)
		return
	}
	if start-end < 1e-6 {
		log.Printf("%v: too short: %.1f - %.1f < 1e-6", subSpan, start, end)
		return
	}

	path := vector.Path{}
	path.MoveTo(offset, start)
	path.LineTo(offset, end)
	path.LineTo(offset+subWidth, end)
	path.LineTo(offset+subWidth, start)
	op := vector.FillOptions{
		Color: spanPalette[subSpan.span.id],
	}
	path.Fill(image, &op)
}

type SubSpan struct {
	span                 *Span
	subindex, subindices int
	start, end           Time
}

func (s *SubSpan) String() string {
	return fmt.Sprintf("%d/%d [%.2f:%.2f]", s.subindex, s.subindices, s.start, s.end)
}

func (s *SubSpan) Validate() error {
	if s.end.After(s.span.end) {
		return fmt.Errorf("end %.2f after parent span %v", s.end.Delta(s.span.end), s.span)
	}
	if s.end.Before(s.span.start) {
		return fmt.Errorf("end %.2f before start of parent span %v", s.span.start.Delta(s.end), s.span)
	}
	if s.start.After(s.span.end) {
		return fmt.Errorf("start %.2f after end of parent span %v", s.end.Delta(s.span.start), s.span)
	}
	if s.start.Before(s.span.start) {
		return fmt.Errorf("start %.2f before parent span %v", s.span.start.Delta(s.start), s.span)
	}
	if s.subindices < 1 {
		return fmt.Errorf("subindices %d < 1", s.subindices)
	}
	if s.subindex >= s.subindices {
		return fmt.Errorf("subindex %d >= %d", s.subindex, s.subindices)
	}
	return nil
}

type SpanEvent struct {
	t     Time
	start bool
	span  *Span
}

func Subindex(spans []*Span) []*SubSpan {
	// Organize span events by position.
	byPos := map[int][]SpanEvent{}
	for _, span := range spans {
		byPos[span.pos] = append(
			byPos[span.pos],
			SpanEvent{t: span.start, start: true, span: span},
			SpanEvent{t: span.end, start: false, span: span},
		)
	}
	for _, events := range byPos {
		sort.Slice(events, func(i, j int) bool { return events[i].t.Before(events[j].t) })
	}

	subSpans := []*SubSpan{}
	active := map[*Span]*SubSpan{}
	var packTime Time
	for _, events := range byPos {
		for i, event := range events {
			hadActive := len(active)

			if event.start {
				active[event.span] = &SubSpan{
					span: event.span, start: event.span.start,
					// This will be overwritten later
					end: event.span.end,
					// These may be updated as needed
					subindex: 0, subindices: 1,
				}
				subSpans = append(subSpans, active[event.span])
			} else {
				active[event.span].end = event.t
				delete(active, event.span)
			}

			// Potentially allow multiple changes before re-indexing.
			if packTime.IsZero() {
				packTime = event.t
			}
			if len(events) > i+1 {
				if packTime.Add(VisualSlack).After(events[i+1].t) {
					continue
				}
			}

			// 0 -> 1 and 1 -> 0
			if hadActive == 0 && len(active) == 0 {
				continue
			}

			// Order current active
			ordered := make([]*SubSpan, len(active))
			i := 0
			for _, subSpan := range active {
				ordered[i] = subSpan
				i++
			}
			sort.Slice(ordered, func(i, j int) bool { return ordered[i].span.id < ordered[j].span.id })
			for i, subSpan := range ordered {
				// If changing an existing span, need to create a cut point here
				if subSpan.start.Before(packTime) {
					// Finish previous subspan
					subSpan.end = packTime
					// Start a new one
					newSubSpan := &SubSpan{
						span: subSpan.span, start: packTime,
						// This will be set right below
						end: Time{}, subindex: 0, subindices: 0,
					}
					active[subSpan.span] = newSubSpan
					subSpans = append(subSpans, newSubSpan)
					// Use the new one below
					subSpan = newSubSpan
				}
				// In any case, renumber the subspans
				subSpan.subindex = i
				subSpan.subindices = len(ordered)
			}

			packTime = Time{}
		}
	}

	for _, subSpan := range subSpans {
		if err := subSpan.Validate(); err != nil {
			log.Printf("Subspan validation failed for %v: %v", subSpan, err)
		}
	}

	return subSpans
}

// Produce a (cached) slice of the trail with spans.
func (trail *Trail) getCached(ctxt context.Context, imageBucketTime Time) *ebiten.Image {
	defer trace.StartRegion(ctxt, "getCachedBucket").End()
	trail.mu.Lock()
	defer trail.mu.Unlock()

	if !trail.cachedReady[imageBucketTime] {
		if trail.cached[imageBucketTime] == nil {
			trail.cached[imageBucketTime] = trail.allocateImage()
		}
		image := trail.cached[imageBucketTime]

		var spans []*Span
		imageBucketEndTime := imageBucketTime.Add(trail.bucketSize)
		image.DrawImage(trail.getGrid(ctxt), &ebiten.DrawImageOptions{})

		for _, bucket := range trail.buckets {
			if err := bucket.Validate(); err != nil {
				log.Fatalf("Invalid bucket: %v", err)
			}
			if !bucket.InRange(imageBucketTime, imageBucketEndTime) {
				//log.Printf("Bucket %v not in range", bucket.start)
				continue
			}

			for _, span := range bucket.spans {
				if !span.InRange(imageBucketTime, imageBucketEndTime) {
					//log.Printf("Span %v not in range", span.start)
					continue
				}
				spans = append(spans, span)
			}
		}

		subSpans := Subindex(spans)
		for _, subSpan := range subSpans {
			trail.drawSubSpan(image, imageBucketTime, subSpan)
		}
		//log.Printf(
		//	"New image slice %dx%d with %d spans for bucket at %v",
		//	image.Bounds().Max.X, image.Bounds().Max.Y, spanCount, imageBucketTime,
		//)

		trail.cachedReady[imageBucketTime] = true
	}
	return trail.cached[imageBucketTime]
}
