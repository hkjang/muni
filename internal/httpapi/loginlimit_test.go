package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestGuessingRunsOutOfAttempts(t *testing.T) {
	limiter := newLoginAttempts()
	const key = "10.0.0.1\x00hong"
	for attempt := 0; attempt < loginBurst; attempt++ {
		if limiter.blocked(key) {
			t.Fatalf("blocked after %d attempts, before the limit", attempt)
		}
		limiter.fail(key)
	}
	if !limiter.blocked(key) {
		t.Fatal("expected the attempts to run out at the limit")
	}
}

func TestGettingInForgetsTheFailures(t *testing.T) {
	// Somebody who mistyped their password four times should not carry that
	// around for the next quarter of an hour.
	limiter := newLoginAttempts()
	const key = "10.0.0.1\x00hong"
	for attempt := 0; attempt < loginBurst-1; attempt++ {
		limiter.fail(key)
	}
	limiter.succeed(key)
	for attempt := 0; attempt < loginBurst-1; attempt++ {
		limiter.fail(key)
	}
	if limiter.blocked(key) {
		t.Fatal("a successful sign-in should have cleared the count")
	}
}

func TestAttemptsAreForgottenAfterTheWindow(t *testing.T) {
	limiter := newLoginAttempts()
	now := time.Now()
	limiter.nowFunc = func() time.Time { return now }
	const key = "10.0.0.1\x00hong"
	for attempt := 0; attempt < loginBurst; attempt++ {
		limiter.fail(key)
	}
	if !limiter.blocked(key) {
		t.Fatal("expected to be blocked")
	}
	now = now.Add(loginWindow + time.Minute)
	if limiter.blocked(key) {
		t.Fatal("the block should not outlive the window")
	}
}

func TestOnePersonCannotLockOutAnother(t *testing.T) {
	// Keying on the account alone would let anyone lock a colleague out by
	// guessing at their name from anywhere.
	limiter := newLoginAttempts()
	attacker := "10.0.0.99\x00hong"
	victim := "10.0.0.1\x00hong"
	for attempt := 0; attempt < loginBurst*2; attempt++ {
		limiter.fail(attacker)
	}
	if !limiter.blocked(attacker) {
		t.Fatal("the attacker should be blocked")
	}
	if limiter.blocked(victim) {
		t.Fatal("the account's real owner should still be able to sign in")
	}
}

func TestOneBadMorningDoesNotBlockAnOffice(t *testing.T) {
	// Keying on the address alone would stop everyone behind one gateway.
	limiter := newLoginAttempts()
	for attempt := 0; attempt < loginBurst*2; attempt++ {
		limiter.fail("10.0.0.1\x00hong")
	}
	if limiter.blocked("10.0.0.1\x00kim") {
		t.Fatal("a colleague at the same address should be unaffected")
	}
}

func TestLoginKeySeparatesAddressAndAccount(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", nil)
	request.RemoteAddr = "192.0.2.10:5555"
	if key := loginKey(request, " Hong "); key != "192.0.2.10\x00hong" {
		t.Fatalf("key = %q", key)
	}
}

func TestAnUnreadableAddressStillCounts(t *testing.T) {
	// Otherwise it would be the way around the limit.
	request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", nil)
	request.RemoteAddr = "not-an-address"
	if key := loginKey(request, "hong"); key != "unknown\x00hong" {
		t.Fatalf("key = %q", key)
	}
}
