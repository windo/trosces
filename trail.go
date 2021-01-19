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

type Span struct {
	// "Instrument" or other category ID (for the same position)
	id int
	// Position/lane on the timeline
	pos int

	// Start of the span.
	start time.Time
	// End - potentially ~far in the future.
	end time.Time
}

func (span *Span) InRange(start time.Time, end time.Time) bool {
	if span.end.Before(start) {
		return false
	}
	if span.start.After(end) {
		return false
	}
	return true
}

func (span *Span) String() string {
	now := time.Now()
	return fmt.Sprintf("%d@%d [%.2f:%.2f]", span.id, span.pos, span.start.Sub(now).Seconds(), span.end.Sub(now).Seconds())
}

var (
	spanPalette = []color.Color{
		color.RGBA{0x44, 0x77, 0xaa, 0xff},
		color.RGBA{0x66, 0xcc, 0xee, 0xff},
		color.RGBA{0x22, 0x88, 0x33, 0xff},
		color.RGBA{0xcc, 0xbb, 0x44, 0xff},
		color.RGBA{0xee, 0x66, 0x77, 0xff},
		color.RGBA{0xaa, 0x33, 0x77, 0xff},
		color.RGBA{0xbb, 0xbb, 0xbb, 0xff},
	}
)

type SpanBucket struct {
	start time.Time
	end   time.Time
	spans []*Span
}

func (bucket *SpanBucket) InRange(start time.Time, end time.Time) bool {
	if bucket.end.Before(start) {
		return false
	}
	if bucket.start.After(end) {
		return false
	}
	return true
}

func (bucket *SpanBucket) UpdateEnd() {
	var latest time.Time
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
	buckets map[time.Time]*SpanBucket

	// Images of spans from [time : time + bucketSize]
	cached      map[time.Time]*ebiten.Image
	cachedReady map[time.Time]bool
	grid        *ebiten.Image
	gridReady   bool
	unused      []*ebiten.Image

	minPos int
	maxPos int

	secondSize float32
	bpm        float32
	gridSteps  int
	length     time.Duration
	bucketSize time.Duration

	posWidth    float32
	borderWidth float32

	mu sync.Mutex
}

func NewTrail(secondSize float32, length time.Duration) *Trail {
	log.Printf("New trail")
	trail := Trail{
		buckets: map[time.Time]*SpanBucket{},

		cached:      map[time.Time]*ebiten.Image{},
		cachedReady: map[time.Time]bool{},
		unused:      []*ebiten.Image{},

		minPos: 0,
		maxPos: 0,

		secondSize:  secondSize,
		bucketSize:  time.Second,
		length:      length,
		borderWidth: 1,
		posWidth:    14,
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

func (trail *Trail) Span(id int, pos int, d time.Duration) {
	defer trace.StartRegion(context.Background(), "NewSpan").End()
	now := time.Now()
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
}

func (trail *Trail) Stop(id int, pos int) {
	defer trace.StartRegion(context.Background(), "StopSpan").End()
	now := time.Now()
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

	now := time.Now()
	activeMap := map[int]struct{}{}
	active := []int{}

	for _, bucket := range trail.buckets {
		if !bucket.InRange(now, now) {
			continue
		}
		for _, span := range bucket.spans {
			if !span.InRange(now, now) {
				continue
			}
			if _, ok := activeMap[span.pos]; !ok {
				activeMap[span.pos] = struct{}{}
				active = append(active, span.pos)
			}
		}
	}
	sort.Ints(active)
	return active
}

// Draw all the trail components.
func (trail *Trail) Draw(ctxt context.Context, image *ebiten.Image, op *ebiten.DrawImageOptions) {
	defer trace.StartRegion(ctxt, "DrawTrail").End()
	now := time.Now()

	// History (time < now) flows away from 0.

	// Bucket covering now, extensing at most bucketSize to the future
	bucketTime := now.Truncate(trail.bucketSize)
	// End of the scroll trail
	trailEnd := now.Add(-trail.length)

	// Until we find a bucket that covers the end of the trail
	for bucketTime.After(trailEnd) {
		bucketOp := ebiten.DrawImageOptions{}
		bucketOp.GeoM = op.GeoM
		// bucket images contain [bucketTime+bucketSize (fresher edge, y=0) ... bucketTime (older edge, y>0)]
		// now -> on screen y=0, future -> on screen y<0
		offset := now.Sub(bucketTime.Add(trail.bucketSize)).Seconds() * float64(trail.secondSize)
		bucketOp.GeoM.Translate(0, offset)
		image.DrawImage(trail.getCached(ctxt, bucketTime), &bucketOp)
		// move to one older bucket
		bucketTime = bucketTime.Add(-trail.bucketSize)
	}
}

// Internal

func (trail *Trail) redrawBucket(bucketTime time.Time) {
	trail.cachedReady[bucketTime] = false
}

func (trail *Trail) redrawAll() {
	trail.cachedReady = map[time.Time]bool{}
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
	trail.cached = map[time.Time]*ebiten.Image{}

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
		int(float32(trail.bucketSize.Seconds())*trail.secondSize),
	)
}

// Discard old spans and cached images
func (trail *Trail) cleanup() {
	_, task := trace.NewTask(context.Background(), "cleanup")
	defer task.End()
	log.Printf("Starting cleanup")

	now := time.Now()

	trail.mu.Lock()
	defer trail.mu.Unlock()

	// Cleanup old span buckets
	removeBuckets := []time.Time{}
	for bucketTime, bucket := range trail.buckets {
		if bucket.end.Before(now.Add(-trail.length)) {
			removeBuckets = append(removeBuckets, bucketTime)
		}
	}
	for _, bucketTime := range removeBuckets {
		delete(trail.buckets, bucketTime)
	}

	// Reuse images
	freeCached := []time.Time{}
	for bucketTime := range trail.cached {
		if bucketTime.Add(trail.bucketSize).Before(now.Add(-trail.length)) {
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
		for t := float32(0); t < float32(trail.bucketSize.Seconds()); t += float32(trail.bucketSize.Seconds() / float64(trail.gridSteps)) {
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

// Produce a (cached) slice of the trail with spans.
func (trail *Trail) getCached(ctxt context.Context, imageBucketTime time.Time) *ebiten.Image {
	defer trace.StartRegion(ctxt, "getCachedBucket").End()
	trail.mu.Lock()
	defer trail.mu.Unlock()

	if !trail.cachedReady[imageBucketTime] {
		if trail.cached[imageBucketTime] == nil {
			trail.cached[imageBucketTime] = trail.allocateImage()
		}
		image := trail.cached[imageBucketTime]

		var spanCount int
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

				// X: note index
				offset := float32(span.pos-trail.minPos) * trail.posWidth

				// start==bucketEndTime -> y=0, older (start < bucketEndTime) -> y>0
				// start < bucketEndTime, no limit vs bucketTime
				start := float32(math.Min(
					imageBucketEndTime.Sub(span.start).Seconds()*float64(trail.secondSize),
					float64(image.Bounds().Max.Y),
				))
				// end > bucketTime, no limit vs bucketEndTime
				end := float32(math.Max(imageBucketEndTime.Sub(span.end).Seconds()*float64(trail.secondSize), 0))

				//log.Printf("Drawing: %v -> [%.1f : %.1f] in %v", span, start, end, imageBucketTime)

				if start < 0 {
					log.Fatalf("start should be within bucket")
				}
				if end > float32(image.Bounds().Max.Y) {
					log.Fatalf("end should be within bucket")
				}
				if end > start {
					log.Fatalf("wrong order")
				}
				if start-end < 1e-6 {
					log.Fatalf("too short")
				}

				path := vector.Path{}
				path.MoveTo(offset+trail.borderWidth, start)
				path.LineTo(offset+trail.borderWidth, end)
				path.LineTo(offset+trail.posWidth-trail.borderWidth, end)
				path.LineTo(offset+trail.posWidth-trail.borderWidth, start)
				op := vector.FillOptions{
					Color: spanPalette[span.id],
				}
				path.Fill(image, &op)
				spanCount++
			}
		}
		//log.Printf(
		//	"New image slice %dx%d with %d spans for bucket at %v",
		//	image.Bounds().Max.X, image.Bounds().Max.Y, spanCount, imageBucketTime,
		//)

		trail.cachedReady[imageBucketTime] = true
	}
	return trail.cached[imageBucketTime]
}
