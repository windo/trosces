package main

import (
	"context"
	"fmt"
	"image/color"
	"log"
	"math"
	"runtime/trace"
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

type Track struct {
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

func NewTrack(secondSize float32, length time.Duration) *Track {
	log.Printf("New track")
	track := Track{
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
			track.cleanup()
		}
	}()

	return &track
}

func (track *Track) Span(id int, pos int, d time.Duration) {
	defer trace.StartRegion(context.Background(), "NewSpan").End()
	now := time.Now()
	bucketTime := now.Truncate(track.bucketSize)

	if id >= len(spanPalette) {
		log.Printf("Instrument ID %d too big, wrapping around", id)
		id %= len(spanPalette)
	}

	track.mu.Lock()
	defer track.mu.Unlock()

	if len(track.buckets) == 0 {
		track.minPos = pos
		track.maxPos = pos
		track.resetAll()
	} else if pos < track.minPos {
		track.minPos = pos
		track.resetAll()
	} else if pos > track.maxPos {
		track.maxPos = pos
		track.resetAll()
	}

	span := &Span{
		id:    id,
		pos:   pos,
		start: now,
		end:   now.Add(d),
	}
	//log.Printf("New span: %s", span.String())

	var bucket *SpanBucket
	if track.buckets[bucketTime] == nil {
		bucket = &SpanBucket{
			start: bucketTime,
			end:   span.end,
		}
		track.buckets[bucketTime] = bucket
	} else {
		bucket = track.buckets[bucketTime]
		if span.end.After(bucket.end) {
			bucket.end = span.end
		}
	}

	// Invalidate cached bucket image
	track.redrawBucket(bucketTime)
	bucket.spans = append(bucket.spans, span)
}

func (track *Track) Stop(id int, pos int) {
	defer trace.StartRegion(context.Background(), "StopSpan").End()
	now := time.Now()
	track.mu.Lock()
	defer track.mu.Unlock()

	for _, bucket := range track.buckets {
		if bucket.end.Before(now) {
			continue
		}
		for _, span := range bucket.spans {
			if span.id == id && span.pos == pos && span.end.After(now) {
				span.end = now
				track.redrawBucket(now.Truncate(track.bucketSize))
				defer bucket.UpdateEnd()
				return
			}
		}
	}
}

func (track *Track) SetGridSteps(steps int) {
	track.mu.Lock()
	defer track.mu.Unlock()

	track.gridSteps = steps
	track.redrawAll()
}

// Draw all the track components.
func (track *Track) Draw(ctxt context.Context, image *ebiten.Image, op *ebiten.DrawImageOptions) {
	defer trace.StartRegion(ctxt, "DrawTrack").End()
	now := time.Now()

	// History (time < now) flows away from 0.

	// Bucket covering now, extensing at most bucketSize to the future
	bucketTime := now.Truncate(track.bucketSize)
	// End of the scroll track
	trackEnd := now.Add(-track.length)

	// Until we find a bucket that covers the end of the track
	for bucketTime.After(trackEnd) {
		bucketOp := ebiten.DrawImageOptions{}
		bucketOp.GeoM = op.GeoM
		// bucket images contain [bucketTime+bucketSize (fresher edge, y=0) ... bucketTime (older edge, y>0)]
		// now -> on screen y=0, future -> on screen y<0
		offset := now.Sub(bucketTime.Add(track.bucketSize)).Seconds() * float64(track.secondSize)
		bucketOp.GeoM.Translate(0, offset)
		image.DrawImage(track.getCached(ctxt, bucketTime), &bucketOp)
		// move to one older bucket
		bucketTime = bucketTime.Add(-track.bucketSize)
	}
}

// Internal

func (track *Track) redrawBucket(bucketTime time.Time) {
	track.cachedReady[bucketTime] = false
}

func (track *Track) redrawAll() {
	track.cachedReady = map[time.Time]bool{}
	track.gridReady = false
}

func (track *Track) resetAll() {
	// mu must be held

	disposeLater := func(image *ebiten.Image) {
		go func() {
			time.Sleep(5 * time.Second)
			image.Dispose()
		}()
	}

	for _, image := range track.cached {
		disposeLater(image)
	}
	track.cached = map[time.Time]*ebiten.Image{}

	if track.grid != nil {
		disposeLater(track.grid)
	}
	track.grid = nil

	for _, image := range track.unused {
		disposeLater(image)
	}
	track.unused = []*ebiten.Image{}

	track.redrawAll()
}

// Discard old spans and cached images
func (track *Track) cleanup() {
	_, task := trace.NewTask(context.Background(), "cleanup")
	defer task.End()
	log.Printf("Starting cleanup")

	now := time.Now()

	track.mu.Lock()
	defer track.mu.Unlock()

	// Cleanup old span buckets
	removeBuckets := []time.Time{}
	for bucketTime, bucket := range track.buckets {
		if bucket.end.Before(now.Add(-track.length)) {
			removeBuckets = append(removeBuckets, bucketTime)
		}
	}
	for _, bucketTime := range removeBuckets {
		delete(track.buckets, bucketTime)
	}

	// Reuse images
	freeCached := []time.Time{}
	for bucketTime := range track.cached {
		if bucketTime.Add(track.bucketSize).Before(now.Add(-track.length)) {
			freeCached = append(freeCached, bucketTime)
		}
	}
	for _, bucketTime := range freeCached {
		log.Printf("Adding %v to unused", bucketTime)
		image := track.cached[bucketTime]
		track.unused = append(track.unused, image)
		delete(track.cached, bucketTime)
	}

	log.Printf("Finished cleanup")
}

// Produce a (cached) grid for track background.
func (track *Track) getGrid(ctxt context.Context) *ebiten.Image {
	defer trace.StartRegion(ctxt, "getCachedGrid").End()
	// mu must be taken
	if !track.gridReady {
		if track.grid == nil {
			track.grid = ebiten.NewImage(
				int(track.posWidth*float32(track.maxPos-track.minPos+1)),
				int(float32(track.bucketSize.Seconds())*track.secondSize),
			)
		}

		// Key columns
		for pos := track.minPos; pos <= track.maxPos; pos++ {
			basePos := pos - track.minPos

			path := vector.Path{}
			path.MoveTo(float32(basePos)*track.posWidth, 0)
			path.LineTo(float32(basePos)*track.posWidth, float32(track.grid.Bounds().Max.Y))
			path.LineTo(float32(basePos+1)*track.posWidth, float32(track.grid.Bounds().Max.Y))
			path.LineTo(float32(basePos+1)*track.posWidth, 0)

			var op vector.FillOptions
			switch {
			case pos%12 == 0:
				op.Color = color.RGBA{0x30, 0x30, 0x30, 0xff}
			case Note(pos).IsWhite():
				op.Color = color.RGBA{0x20, 0x20, 0x20, 0xff}
			default:
				op.Color = color.RGBA{0x10, 0x10, 0x10, 0xff}
			}

			path.Fill(track.grid, &op)
		}

		// Timeline
		for t := float32(0); t < float32(track.bucketSize.Seconds()); t += float32(track.bucketSize.Seconds() / float64(track.gridSteps)) {
			baseTime := t * track.secondSize

			path := vector.Path{}
			path.MoveTo(0, baseTime)
			path.LineTo(float32(track.grid.Bounds().Max.X), baseTime)
			path.LineTo(float32(track.grid.Bounds().Max.X), baseTime+track.borderWidth)
			path.LineTo(0, baseTime+track.borderWidth)

			var op vector.FillOptions
			switch {
			case baseTime == 0:
				op.Color = color.RGBA{0xc0, 0xc0, 0xc0, 0xff}
			default:
				op.Color = color.RGBA{0x80, 0x80, 0x80, 0xff}
			}

			path.Fill(track.grid, &op)
		}

		// Updated!
		track.gridReady = true
	}
	return track.grid
}

// Produce a (cached) slice of the track with spans.
func (track *Track) getCached(ctxt context.Context, imageBucketTime time.Time) *ebiten.Image {
	defer trace.StartRegion(ctxt, "getCachedBucket").End()
	track.mu.Lock()
	defer track.mu.Unlock()

	if !track.cachedReady[imageBucketTime] {
		var image *ebiten.Image
		if track.cached[imageBucketTime] == nil {
			if len(track.unused) > 0 {
				log.Printf("Reusing unused (%d)", len(track.unused))
				image = track.unused[len(track.unused)-1]
				track.unused = track.unused[:len(track.unused)-1]
				log.Printf("image: %T %v", image, image)
				image.Fill(color.Black)
			} else {
				log.Printf("Creating new")
				image = ebiten.NewImage(
					int(track.posWidth*float32(track.maxPos-track.minPos+1)),
					int(float32(track.bucketSize.Seconds())*track.secondSize),
				)
			}
			track.cached[imageBucketTime] = image
		} else {
			image = track.cached[imageBucketTime]
		}

		var spanCount int
		imageBucketEndTime := imageBucketTime.Add(track.bucketSize)
		image.DrawImage(track.getGrid(ctxt), &ebiten.DrawImageOptions{})

		for _, bucket := range track.buckets {
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
				offset := float32(span.pos-track.minPos) * track.posWidth

				// start==bucketEndTime -> y=0, older (start < bucketEndTime) -> y>0
				// start < bucketEndTime, no limit vs bucketTime
				start := float32(math.Min(
					imageBucketEndTime.Sub(span.start).Seconds()*float64(track.secondSize),
					float64(image.Bounds().Max.Y),
				))
				// end > bucketTime, no limit vs bucketEndTime
				end := float32(math.Max(imageBucketEndTime.Sub(span.end).Seconds()*float64(track.secondSize), 0))

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
				path.MoveTo(offset+track.borderWidth, start)
				path.LineTo(offset+track.borderWidth, end)
				path.LineTo(offset+track.posWidth-track.borderWidth, end)
				path.LineTo(offset+track.posWidth-track.borderWidth, start)
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

		track.cachedReady[imageBucketTime] = true
	}
	return track.cached[imageBucketTime]
}
