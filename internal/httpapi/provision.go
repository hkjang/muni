package httpapi

import (
	"context"
	"crypto/rand"
	"fmt"
	"math/big"
	"strings"

	"github.com/google/uuid"
	"github.com/hkjang/muni/internal/cryptoutil"
	"github.com/jackc/pgx/v5"
)

// An account is not one row. It is a user, the personal workspace that user
// writes in, the membership that connects them, and the wrapped data key that
// makes the workspace readable. Creating three of the four leaves an account
// that signs in and then cannot do anything, so all four happen in one
// transaction or none do.
//
// This used to exist only inside the OIDC callback, which is why an office
// that does not run OIDC had exactly one account — the one the container
// created at boot — and no way to make a second.

type provisionSpec struct {
	Username     string
	Email        string
	DisplayName  string
	Role         string
	Locale       string
	PasswordHash *string
	OIDCSubject  *string
	AvatarURL    string
	// MustChangePassword marks a password muni or an administrator chose
	// rather than the person who will use it.
	MustChangePassword bool
	CreatedBy          *uuid.UUID
}

// provisionUser creates the account inside the caller's transaction and
// returns the user, with Username set to whatever name it actually got.
func provisionUser(ctx context.Context, tx pgx.Tx, sealer *cryptoutil.Sealer, spec provisionSpec) (User, error) {
	userID := uuid.New()
	dataKey, err := cryptoutil.RandomBytes(32)
	if err != nil {
		return User{}, err
	}
	wrapped, err := sealer.Seal(dataKey, "user-key:"+userID.String()+":1")
	if err != nil {
		return User{}, err
	}

	// normalizeUsername never returns an empty string — given nothing it
	// invents a placeholder — so the fallback to the email has to happen
	// before normalizing, not after. Otherwise every account created from an
	// email alone is named after the same placeholder, and the second one
	// collides with the first.
	source := strings.TrimSpace(spec.Username)
	if source == "" {
		source = strings.Split(spec.Email, "@")[0]
	}
	username := normalizeUsername(source)
	displayName := strings.TrimSpace(spec.DisplayName)
	if displayName == "" {
		displayName = username
	}
	role := spec.Role
	if role != "ADMIN" {
		role = "USER"
	}
	locale := strings.TrimSpace(spec.Locale)
	if locale == "" {
		locale = "ko-KR"
	}

	// Two people named 김민수 both want kimminsu. The second gets kimminsu-2
	// rather than an error, because an administrator importing a staff list
	// cannot be asked to invent usernames for the collisions.
	// Each attempt runs inside its own savepoint. A failed INSERT poisons the
	// whole transaction in PostgreSQL — every later statement answers "current
	// transaction is aborted" — so retrying straight after the failure never
	// worked. It looked like it did, because until an administrator could
	// create accounts the only caller was OIDC, and two people whose email
	// prefixes match is rare enough that nobody hit it.
	candidate := username
	var inserted bool
	for attempt := 0; attempt < 20; attempt++ {
		nested, err := tx.Begin(ctx)
		if err != nil {
			return User{}, err
		}
		_, err = nested.Exec(ctx, `INSERT INTO users(id,username,email,display_name,password_hash,role,status,oidc_subject,avatar_url,locale,password_reset_required,created_by)
			VALUES($1,$2,$3,$4,$5,$6,'ACTIVE',$7,NULLIF($8,''),$9,$10,$11)`,
			userID, candidate, strings.ToLower(strings.TrimSpace(spec.Email)), displayName,
			spec.PasswordHash, role, spec.OIDCSubject, spec.AvatarURL, locale,
			spec.MustChangePassword, spec.CreatedBy)
		if err == nil {
			if err := nested.Commit(ctx); err != nil {
				return User{}, err
			}
			username = candidate
			inserted = true
			break
		}
		_ = nested.Rollback(ctx)
		if !strings.Contains(err.Error(), "users_username_key") {
			return User{}, err
		}
		candidate = fmt.Sprintf("%s-%d", username, attempt+2)
	}
	if !inserted {
		return User{}, fmt.Errorf("could not find a free username near %q", username)
	}

	workspaceID := uuid.New()
	if _, err := tx.Exec(ctx, `INSERT INTO workspaces(id,name,slug,kind,owner_id) VALUES($1,$2,$3,'PERSONAL',$4)`,
		workspaceID, displayName+"의 문서", "personal-"+userID.String(), userID); err != nil {
		return User{}, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO workspace_members(workspace_id,user_id,role) VALUES($1,$2,'OWNER')`, workspaceID, userID); err != nil {
		return User{}, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO user_keys(user_id,name,fingerprint,wrapped_key,status,version) VALUES($1,'기본 개인 키',$2,$3,'ACTIVE',1)`,
		userID, cryptoutil.Fingerprint(dataKey), wrapped); err != nil {
		return User{}, err
	}

	return User{
		ID:          userID,
		Username:    username,
		Email:       strings.ToLower(strings.TrimSpace(spec.Email)),
		DisplayName: displayName,
		Role:        role,
		Status:      "ACTIVE",
		Locale:      locale,
	}, nil
}

// Characters that are hard to confuse when a password is read off a screen and
// typed on another machine, or read aloud over a desk. No O/0, no l/1/I, and
// nothing that a Korean keyboard layout makes awkward to reach.
const passwordAlphabet = "abcdefghijkmnpqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ23456789"

// generatePassword returns a password muni chose. It is long rather than
// clever: the person holding it is expected to replace it on first sign-in, so
// the only job is surviving the trip from the administrator to them.
func generatePassword() (string, error) {
	const length = 16
	max := big.NewInt(int64(len(passwordAlphabet)))
	var b strings.Builder
	for i := 0; i < length; i++ {
		n, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", err
		}
		b.WriteByte(passwordAlphabet[n.Int64()])
		// Grouped for reading. The dashes count toward nothing and the
		// alphabet excludes them, so they cannot be confused for content.
		if i%4 == 3 && i != length-1 {
			b.WriteByte('-')
		}
	}
	return b.String(), nil
}
