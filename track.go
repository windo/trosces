package main

import (
	"fmt"
	"image/color"
	"log"
	"math"
	"sync"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

type Trace struct {
	// "Instrument" or other category ID (for the same position)
	id int
	// Position/lane on the timeline
	pos int

	// Start of the trace.
	start time.Time
	// End - potentially ~far in the future.
	end time.Time
}

func (trace *Trace) InRange(start time.Time, end time.Time) bool {
	if trace.end.Before(start) {
		return false
	}
	if trace.start.After(end) {
		return false
	}
	return true
}

func (trace *Trace) String() string {
	now := time.Now()
	return fmt.Sprintf("%d@%d [%.2f:%.2f]", trace.id, trace.pos, trace.start.Sub(now).Seconds(), trace.end.Sub(now).Seconds())
}

var (
	tracePalette = []color.Color{
		color.RGBA{0x44, 0x77, 0xaa, 0xff},
		color.RGBA{0x66, 0xcc, 0xee, 0xff},
		color.RGBA{0x22, 0x88, 0x33, 0xff},
		color.RGBA{0xcc, 0xbb, 0x44, 0xff},
		color.RGBA{0xee, 0x66, 0x77, 0xff},
		color.RGBA{0xaa, 0x33, 0x77, 0xff},
		color.RGBA{0xbb, 0xbb, 0xbb, 0xff},
	}
)

type TraceBucket struct {
	start  time.Time
	end    time.Time
	traces []*Trace
}

func (bucket *TraceBucket) InRange(start time.Time, end time.Time) bool {
	if bucket.end.Before(start) {
		return false
	}
	if bucket.start.After(end) {
		return false
	}
	return true
}

func (bucket *TraceBucket) UpdateEnd() {
	var latest time.Time
	for _, trace := range bucket.traces {
		if trace.end.After(latest) {
			latest = trace.end
		}
	}
	bucket.end = latest
}

func (bucket *TraceBucket) Validate() error {
	for _, trace := range bucket.traces {
		if trace.start.Before(bucket.start) || trace.start.After(bucket.end) || trace.end.Before(bucket.start) {
			return fmt.Errorf("Out of bucket %v trace: %v!", bucket, trace)
		}
	}
	return nil
}

type Track struct {
	// Traces bucketed by periods of duration `bucketSize`
	buckets map[time.Time]*TraceBucket
	// Images of traces from [time : time + bucketSize]
	cached map[time.Time]*ebiten.Image

	minPos int
	maxPos int

	secondSize float32
	bpm        float32
	length     time.Duration
	bucketSize time.Duration

	posWidth    float32
	borderWidth float32

	mu sync.Mutex
}

func NewTrack(secondSize float32, length time.Duration) *Track {
	log.Printf("New track")
	track := Track{
		buckets: map[time.Time]*TraceBucket{},
		cached:  map[time.Time]*ebiten.Image{},

		minPos: 0,
		maxPos: 0,

		secondSize:  secondSize,
		bucketSize:  time.Second,
		length:      length,
		borderWidth: 1,
		posWidth:    14,
	}
	return &track
}

func (track *Track) Trace(id int, pos int, d time.Duration) {
	now := time.Now()
	bucketTime := now.Truncate(track.bucketSize)

	if id >= len(tracePalette) {
		log.Printf("Instrument ID %d too big, wrapping around", id)
		id %= len(tracePalette)
	}

	track.mu.Lock()
	defer track.mu.Unlock()

	if len(track.buckets) == 0 {
		track.minPos = pos
		track.maxPos = pos
	} else if pos < track.minPos {
		track.minPos = pos
	} else if pos > track.maxPos {
		track.maxPos = pos
	}

	trace := &Trace{
		id:    id,
		pos:   pos,
		start: now,
		end:   now.Add(d),
	}
	log.Printf("New trace: %s", trace.String())

	var bucket *TraceBucket
	if track.buckets[bucketTime] == nil {
		bucket = &TraceBucket{
			start: bucketTime,
			end:   trace.end,
		}
		track.buckets[bucketTime] = bucket
	} else {
		bucket = track.buckets[bucketTime]
		if trace.end.After(bucket.end) {
			bucket.end = trace.end
		}
	}

	// Invalidate cached bucket image
	delete(track.cached, bucketTime)

	bucket.traces = append(bucket.traces, trace)
}

func (track *Track) Stop(id int, pos int) {
	now := time.Now()
	track.mu.Lock()
	defer track.mu.Unlock()

	for _, bucket := range track.buckets {
		if bucket.end.Before(now) {
			continue
		}
		for _, trace := range bucket.traces {
			if trace.id == id && trace.pos == pos && trace.end.After(now) {
				trace.end = now
				defer bucket.UpdateEnd()
				return
			}
		}
	}
}

func (track *Track) getCached(imageBucketTime time.Time) *ebiten.Image {
	track.mu.Lock()
	defer track.mu.Unlock()

	if track.cached[imageBucketTime] == nil {
		image := ebiten.NewImage(
			int(track.posWidth*float32(track.maxPos-track.minPos+1)),
			int(float32(track.bucketSize.Seconds())*track.secondSize),
		)
		// TODO: getGrid
		if imageBucketTime.Second()%2 == 1 {
			log.Printf("White")
			image.Fill(color.White)
		} else {
			log.Printf("Black")
			image.Fill(color.Black)
		}
		imageBucketEndTime := imageBucketTime.Add(track.bucketSize)

		var traceCount int

		log.Printf("New image: %v -> %v", imageBucketTime, imageBucketEndTime)

		for _, bucket := range track.buckets {
			if err := bucket.Validate(); err != nil {
				log.Fatalf("Invalid bucket: %v", err)
			}
			if !bucket.InRange(imageBucketTime, imageBucketEndTime) {
				log.Printf("Bucket %v not in range", bucket.start)
				continue
			}

			for _, trace := range bucket.traces {
				if !trace.InRange(imageBucketTime, imageBucketEndTime) {
					log.Printf("Trace %v not in range", trace.start)
					continue
				}

				// X: note index
				offset := float32(trace.pos-track.minPos) * track.posWidth

				// start==bucketEndTime -> y=0, older (start < bucketEndTime) -> y>0
				// start < bucketEndTime, no limit vs bucketTime
				start := float32(math.Min(
					imageBucketEndTime.Sub(trace.start).Seconds()*float64(track.secondSize),
					float64(image.Bounds().Max.Y),
				))
				// end > bucketTime, no limit vs bucketEndTime
				end := float32(math.Max(imageBucketEndTime.Sub(trace.end).Seconds()*float64(track.secondSize), 0))

				log.Printf("Drawing: %v -> [%.1f : %.1f] in %v", trace, start, end, imageBucketTime)

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
					Color: tracePalette[trace.id],
				}
				path.Fill(image, &op)
				traceCount++
			}
		}
		log.Printf(
			"New image slice %dx%d with %d traces for bucket at %v",
			image.Bounds().Max.X, image.Bounds().Max.Y, traceCount, imageBucketTime,
		)

		track.cached[imageBucketTime] = image
	}
	return track.cached[imageBucketTime]
}

func (track *Track) Draw(image *ebiten.Image) {
	now := time.Now()

	// History (time < now) flows away from 0.

	// Bucket covering now, extensing at most bucketSize to the future
	bucketTime := now.Truncate(track.bucketSize)
	// End of the scroll track
	trackEnd := now.Add(-track.length)

	// Until we find a bucket that covers the end of the track
	for bucketTime.After(trackEnd) {
		op := ebiten.DrawImageOptions{}
		// bucket images contain [bucketTime+bucketSize (fresher edge, y=0) ... bucketTime (older edge, y>0)]
		// now -> on screen y=0, future -> on screen y<0
		offset := now.Sub(bucketTime.Add(track.bucketSize)).Seconds() * float64(track.secondSize)
		op.GeoM.Translate(0, offset)
		image.DrawImage(track.getCached(bucketTime), &op)
		// move to one older bucket
		bucketTime = bucketTime.Add(-track.bucketSize)
	}

}
