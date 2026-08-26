package httpapi

import (
	"net/http"
	"strings"
	"sync"
	"time"
)

// Repeated failed sign-ins.
//
// A wrong password cost 200 milliseconds and nothing else, which slows one
// attempt at a time and not a hundred sent at once — the delay is served in
// parallel. Counting failures is what actually makes guessing expensive.
//
// The count is held in memory rather than in the database: it is a defence
// against a burst, it should not survive a restart any longer than the burst
// does, and a lockout written to a shared table is a way for one person to
// lock out another by guessing at their name.

// loginWindow is how long failures are remembered.
const loginWindow = 15 * time.Minute

// loginBurst is how many failures are allowed in that window before the
// attempts are refused. It is set well above what a person mistyping their own
// password reaches and far below what guessing needs.
const loginBurst = 10

type loginAttempts struct {
	mu      sync.Mutex
	byKey   map[string]*attemptRecord
	lastGC  time.Time
	nowFunc func() time.Time
}

type attemptRecord struct {
	failures int
	first    time.Time
	last     time.Time
}

func newLoginAttempts() *loginAttempts {
	return &loginAttempts{byKey: map[string]*attemptRecord{}, nowFunc: time.Now}
}

func (l *loginAttempts) now() time.Time {
	if l.nowFunc != nil {
		return l.nowFunc()
	}
	return time.Now()
}

// blocked reports whether this source has run out of attempts.
func (l *loginAttempts) blocked(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	record := l.byKey[key]
	if record == nil {
		return false
	}
	if l.now().Sub(record.first) > loginWindow {
		delete(l.byKey, key)
		return false
	}
	return record.failures >= loginBurst
}

// fail records one failed attempt.
func (l *loginAttempts) fail(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.collect()
	record := l.byKey[key]
	if record == nil || l.now().Sub(record.first) > loginWindow {
		l.byKey[key] = &attemptRecord{failures: 1, first: l.now(), last: l.now()}
		return
	}
	record.failures++
	record.last = l.now()
}

// succeed forgets the failures once the person gets in, so someone who
// mistyped their password four times is not carrying that around.
func (l *loginAttempts) succeed(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.byKey, key)
}

// collect drops records that have aged out. It runs on writes rather than on a
// timer so an idle service keeps no goroutine for it.
func (l *loginAttempts) collect() {
	now := l.now()
	if now.Sub(l.lastGC) < loginWindow {
		return
	}
	l.lastGC = now
	for key, record := range l.byKey {
		if now.Sub(record.first) > loginWindow {
			delete(l.byKey, key)
		}
	}
}

// loginKey identifies where an attempt came from.
//
// The address and the account are combined so that guessing at one account
// from one machine is what gets blocked. Keying on the account alone would let
// anyone lock a colleague out by guessing at their name; keying on the address
// alone would block a whole office behind one gateway because of one person's
// bad morning.
func loginKey(r *http.Request, identity string) string {
	address, _ := clientIP(r).(string)
	if address == "" {
		// An address muni could not read still has to count as something, or
		// it would be the way around the limit.
		address = "unknown"
	}
	return address + "\x00" + strings.ToLower(strings.TrimSpace(identity))
}
