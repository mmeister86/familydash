package calendar

import (
	"context"
	"time"
)

// Source is anything the wall can render calendars from: the local ICS
// poller (Service) or the central Companion backend (familyapp
// CalendarService). The indirection keeps both modes behind one factory and
// guarantees the Convex mode never starts local polling.
type Source interface {
	Snapshot() Snapshot
	Refresh(context.Context)
	Run(context.Context, time.Duration)
}

// Local polling stays available through the same interface.
var _ Source = (*Service)(nil)
