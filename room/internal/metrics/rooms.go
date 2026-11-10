// Package metrics exports bounded room metrics without exposing user or room IDs.
package metrics

import (
	"errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/vexner67/freenet/room/internal/room"
	"time"
)

type Rooms struct {
	live     *prometheus.GaugeVec
	members  prometheus.Gauge
	joins    prometheus.Counter
	failures *prometheus.CounterVec
	sessions prometheus.Histogram
	playback prometheus.Counter
}

func New(reg prometheus.Registerer) *Rooms {
	m := &Rooms{
		live:     prometheus.NewGaugeVec(prometheus.GaugeOpts{Name: "room_live", Help: "Currently active rooms by kind."}, []string{"kind"}),
		members:  prometheus.NewGauge(prometheus.GaugeOpts{Name: "room_members", Help: "Current participants across rooms."}),
		joins:    prometheus.NewCounter(prometheus.CounterOpts{Name: "room_joins_total", Help: "Successful join attempts, including idempotent refreshes."}),
		failures: prometheus.NewCounterVec(prometheus.CounterOpts{Name: "room_join_failures_total", Help: "Failed join attempts by bounded reason."}, []string{"reason"}),
		sessions: prometheus.NewHistogram(prometheus.HistogramOpts{Name: "room_session_seconds", Help: "Completed room participation duration.", Buckets: []float64{5, 30, 60, 300, 900, 1800, 3600, 7200, 14400}}),
		playback: prometheus.NewCounter(prometheus.CounterOpts{Name: "room_playback_changes_total", Help: "Committed shared screen changes."}),
	}
	reg.MustRegister(m.live, m.members, m.joins, m.failures, m.sessions, m.playback)
	for _, kind := range []string{"talk", "watch", "listen"} {
		m.live.WithLabelValues(kind).Set(0)
	}
	return m
}
func (m *Rooms) Join(err error) {
	if err == nil {
		m.joins.Inc()
		return
	}
	reason := "internal"
	switch {
	case errors.Is(err, room.ErrFull):
		reason = "full"
	case errors.Is(err, room.ErrTooManyRooms):
		reason = "capacity"
	case errors.Is(err, room.ErrForbidden):
		reason = "forbidden"
	case errors.Is(err, room.ErrNotFound), errors.Is(err, room.ErrClosed):
		reason = "closed"
	}
	m.failures.WithLabelValues(reason).Inc()
}
func (m *Rooms) Leave(duration time.Duration) {
	if duration >= 0 {
		m.sessions.Observe(duration.Seconds())
	}
}
func (m *Rooms) Playback() { m.playback.Inc() }
func (m *Rooms) Snapshot(kinds map[room.Kind]int, members int) {
	for _, kind := range []room.Kind{room.KindTalk, room.KindWatch, room.KindListen} {
		m.live.WithLabelValues(kind.String()).Set(float64(kinds[kind]))
	}
	m.members.Set(float64(members))
}
