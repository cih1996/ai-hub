package core

import (
	"ai-hub/server/model"
	"ai-hub/server/store"
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"
)

const timeLayout = "2006-01-02 15:04:05"

var bjLoc = time.FixedZone("CST", 8*3600)

type triggerKind int

const (
	kindExact    triggerKind = iota // "2026-02-17 10:30:00"
	kindDaily                       // "10:30:00"
	kindInterval                    // "1h30m"
	kindInvalid
)

func parseTriggerKind(s string) triggerKind {
	if _, err := time.ParseInLocation(timeLayout, s, bjLoc); err == nil {
		return kindExact
	}
	if _, err := time.Parse("15:04:05", s); err == nil {
		return kindDaily
	}
	if _, err := time.ParseDuration(s); err == nil {
		return kindInterval
	}
	return kindInvalid
}

func CalcNextFireAt(t *model.Trigger, now time.Time) string {
	switch parseTriggerKind(t.TriggerTime) {
	case kindExact:
		return t.TriggerTime
	case kindDaily:
		parsed, _ := time.Parse("15:04:05", t.TriggerTime)
		fire := time.Date(now.Year(), now.Month(), now.Day(), parsed.Hour(), parsed.Minute(), parsed.Second(), 0, bjLoc)
		if now.After(fire) {
			fire = fire.AddDate(0, 0, 1)
		}
		return fire.Format(timeLayout)
	case kindInterval:
		dur, _ := time.ParseDuration(t.TriggerTime)
		var base time.Time
		if t.LastFiredAt != "" {
			base, _ = time.ParseInLocation(timeLayout, t.LastFiredAt, bjLoc)
		} else if t.CreatedAt != "" {
			base, _ = time.ParseInLocation(timeLayout, t.CreatedAt, bjLoc)
		} else {
			base = now
		}
		next := base.Add(dur)
		for next.Before(now) {
			next = next.Add(dur)
		}
		return next.Format(timeLayout)
	}
	return ""
}

func shouldFire(t *model.Trigger, now time.Time) bool {
	if !t.Enabled || t.Status == "disabled" || t.Status == "completed" {
		return false
	}
	if t.MaxFires > 0 && t.FiredCount >= t.MaxFires {
		return false
	}
	if t.NextFireAt == "" {
		return false
	}
	nextTime, err := time.ParseInLocation(timeLayout, t.NextFireAt, bjLoc)
	if err != nil {
		return false
	}
	return !now.Before(nextTime)
}

func fireTrigger(t *model.Trigger, port int) error {
	url := fmt.Sprintf("http://localhost:%d/api/v1/chat/send", port)
	body, _ := json.Marshal(map[string]interface{}{
		"session_id": t.SessionID,
		"content":    t.Content,
	})
	resp, err := http.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("http error: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == 404 {
		return fmt.Errorf("session %d not found", t.SessionID)
	}
	if resp.StatusCode == 409 {
		return fmt.Errorf("session %d busy", t.SessionID)
	}
	if resp.StatusCode != 200 {
		return fmt.Errorf("status %d", resp.StatusCode)
	}
	return nil
}

func StartTriggerLoop(port int) {
	ticker := time.NewTicker(1 * time.Minute)
	go func() {
		checkTriggers(port)
		for range ticker.C {
			checkTriggers(port)
		}
	}()
	log.Printf("[trigger] scheduler started, checking every 1 minute")
}

func checkTriggers(port int) {
	triggers, err := store.ListTriggers()
	if err != nil {
		log.Printf("[trigger] list error: %v", err)
		return
	}
	if len(triggers) == 0 {
		return
	}

	now := time.Now().In(bjLoc)

	for i := range triggers {
		t := &triggers[i]
		dirty := false

		// 同步 enabled <-> status
		if !t.Enabled && t.Status != "disabled" {
			t.Status = "disabled"
			dirty = true
		}
		if t.Enabled && t.Status == "disabled" {
			t.Status = "active"
			dirty = true
		}
		if t.Status == "fired" {
			t.Status = "active"
			dirty = true
		}

		// 先用当前 next_fire_at 判断是否该触发（不要提前刷新）
		if shouldFire(t, now) {
			log.Printf("[trigger] firing: id=%d session=%d", t.ID, t.SessionID)
			if err := fireTrigger(t, port); err != nil {
				log.Printf("[trigger] failed: id=%d err=%v", t.ID, err)
				t.Status = "failed"
				store.UpdateTrigger(t)
				continue
			}

			t.FiredCount++
			t.LastFiredAt = now.Format(timeLayout)
			t.Status = "fired"
			if t.MaxFires > 0 && t.FiredCount >= t.MaxFires {
				t.Status = "completed"
			}
			dirty = true
		}

		// 触发后（或未触发）再刷新 next_fire_at，为下一轮做准备
		newNext := CalcNextFireAt(t, now)
		if newNext != t.NextFireAt {
			t.NextFireAt = newNext
			dirty = true
		}

		if dirty {
			store.UpdateTrigger(t)
		}
	}
}
