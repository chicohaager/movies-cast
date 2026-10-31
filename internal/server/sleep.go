package server

import (
	"context"
	"log"
	"time"

	"movies-cast/internal/dlna"
)

// sleepTimer stops the TV at a set time (the phone's "Sleep timer").
type sleepTimer struct {
	at    time.Time
	timer *time.Timer
}

const maxSleep = 4 * time.Hour

// setSleep starts, replaces or (d <= 0) clears the TV's sleep timer. When it
// fires the TV gets Stop and the play ends like Stop on the phone: nothing
// from the queue follows.
func (s *Server) setSleep(tv dlna.Renderer, d time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if old := s.sleep[tv.ID]; old != nil {
		old.timer.Stop()
		delete(s.sleep, tv.ID)
	}
	if d <= 0 {
		return
	}
	d = min(d, maxSleep)
	st := &sleepTimer{at: s.now().Add(d)}
	st.timer = time.AfterFunc(d, func() {
		s.mu.Lock()
		if s.sleep[tv.ID] != st {
			s.mu.Unlock()
			return
		}
		delete(s.sleep, tv.ID)
		if p := s.byTV[tv.ID]; p != nil {
			s.endPlayLocked(p.id)
		}
		s.mu.Unlock()
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := s.tv.Stop(ctx, tv); err != nil {
			log.Printf("sleep timer: stop %s: %v", tv.Name, err)
			return
		}
		log.Printf("sleep timer: %s stopped", tv.Name)
	})
	s.sleep[tv.ID] = st
}
