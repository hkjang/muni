package httpapi

import "testing"

func TestPasswordMustBeLongEnough(t *testing.T) {
	// The same rule the first administrator's password has to meet at boot.
	if err := checkPassword("짧은비번"); err == nil {
		t.Fatal("a short password should be refused")
	}
	if err := checkPassword("충분히 긴 비밀번호입니다"); err != nil {
		t.Fatalf("a long enough password was refused: %v", err)
	}
}

func TestPasswordLengthCountsCharactersNotBytes(t *testing.T) {
	// Twelve Hangul characters are thirty-six bytes; counting bytes would let
	// a four-character password through.
	if err := checkPassword("가나다라마바사아자차카타"); err != nil {
		t.Fatalf("twelve characters should be accepted: %v", err)
	}
	if err := checkPassword("가나다라"); err == nil {
		t.Fatal("four characters should be refused whatever they weigh")
	}
}

func TestPasswordCannotBeOnlySpaces(t *testing.T) {
	if err := checkPassword("                "); err == nil {
		t.Fatal("whitespace alone should be refused")
	}
}

func TestPasswordHasACeiling(t *testing.T) {
	long := make([]rune, 300)
	for index := range long {
		long[index] = 'a'
	}
	if err := checkPassword(string(long)); err == nil {
		t.Fatal("an absurdly long password should be refused")
	}
}
