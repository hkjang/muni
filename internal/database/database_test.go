package database

import "testing"

func TestPasswordHash(t *testing.T) {
	encoded, err := HashPassword("correct-horse-battery-staple")
	if err != nil {
		t.Fatal(err)
	}
	if !VerifyPassword(encoded, "correct-horse-battery-staple") {
		t.Fatal("correct password was rejected")
	}
	if VerifyPassword(encoded, "incorrect-password") {
		t.Fatal("incorrect password was accepted")
	}
}
