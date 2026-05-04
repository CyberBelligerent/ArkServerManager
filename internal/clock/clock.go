// For scheduling
package clock

import "time"

type Clock interface {
	Now() time.Time
	Sleep(d time.Duration)
}

func System() Clock { return systemClock{} }

type systemClock struct{}

func (systemClock) Now() time.Time        { return time.Now() }
func (systemClock) Sleep(d time.Duration) { time.Sleep(d) }
