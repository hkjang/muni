package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"testing"
)

type userPage struct {
	Data struct {
		Items []struct {
			Email              string  `json:"email"`
			Role               string  `json:"role"`
			Status             string  `json:"status"`
			MustChangePassword bool    `json:"mustChangePassword"`
			LastLoginAt        *string `json:"lastLoginAt"`
		} `json:"items"`
		Total  int `json:"total"`
		Limit  int `json:"limit"`
		Offset int `json:"offset"`
	} `json:"data"`
}

func listUsersWith(t *testing.T, srv *serverUnderTest, query string) userPage {
	t.Helper()
	resp, err := srv.admin.Get(srv.URL + "/api/v1/admin/users?" + query)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("list?%s = %d", query, resp.StatusCode)
	}
	var page userPage
	if err := json.NewDecoder(resp.Body).Decode(&page); err != nil {
		t.Fatal(err)
	}
	return page
}

func TestTheUserListHasASecondPage(t *testing.T) {
	// The old list was one LIMIT with no way past it, so an office of three
	// hundred showed fifty people and no indication there were more.
	srv := newServerUnderTest(t)
	for i := 0; i < 12; i++ {
		createAccount(t, srv, fmt.Sprintf("page%02d@example.com", i), fmt.Sprintf("사람%02d", i))
	}
	first := listUsersWith(t, srv, "limit=5&sort=name")
	if len(first.Data.Items) != 5 {
		t.Fatalf("got %d items", len(first.Data.Items))
	}
	if first.Data.Total < 13 {
		t.Fatalf("total = %d; it must count every match, not the page", first.Data.Total)
	}
	second := listUsersWith(t, srv, "limit=5&offset=5&sort=name")
	if len(second.Data.Items) != 5 {
		t.Fatalf("second page has %d items", len(second.Data.Items))
	}
	// Overlapping pages would silently hide people. The tiebreak on id is
	// what stops rows with equal sort keys from drifting between pages.
	seen := map[string]bool{}
	for _, item := range append(first.Data.Items, second.Data.Items...) {
		if seen[item.Email] {
			t.Fatalf("%s appeared on both pages", item.Email)
		}
		seen[item.Email] = true
	}
}

func TestTheFiltersAnswerRealQuestions(t *testing.T) {
	srv := newServerUnderTest(t)
	ctx := context.Background()
	arrived := createAccount(t, srv, "arrived@example.com", "온사람")
	createAccount(t, srv, "invited@example.com", "초대만된사람")
	boss := createAccount(t, srv, "boss@example.com", "관리자")

	// One person signed in long ago and changed their password; one was
	// invited and never came back.
	if _, err := srv.db.Exec(ctx, `UPDATE users SET last_login_at=now()-interval '200 days',
		password_reset_required=false WHERE id=$1`, arrived); err != nil {
		t.Fatal(err)
	}
	if _, err := srv.db.Exec(ctx, `UPDATE users SET role='ADMIN' WHERE id=$1`, boss); err != nil {
		t.Fatal(err)
	}

	t.Run("who was invited and never arrived", func(t *testing.T) {
		page := listUsersWith(t, srv, "pendingPassword=true&q=example.com")
		for _, item := range page.Data.Items {
			if !item.MustChangePassword {
				t.Fatalf("%s does not hold a temporary password", item.Email)
			}
		}
		if !containsEmail(page, "invited@example.com") || containsEmail(page, "arrived@example.com") {
			t.Fatalf("wrong set: %+v", page.Data.Items)
		}
	})

	t.Run("who has not signed in for a hundred days", func(t *testing.T) {
		page := listUsersWith(t, srv, "inactiveDays=100&q=arrived@example.com")
		if !containsEmail(page, "arrived@example.com") {
			t.Fatal("a login 200 days ago is inactive at a 100 day threshold")
		}
		page = listUsersWith(t, srv, "inactiveDays=300&q=arrived@example.com")
		if containsEmail(page, "arrived@example.com") {
			t.Fatal("a login 200 days ago is not inactive at a 300 day threshold")
		}
	})

	t.Run("who never signed in at all counts as inactive", func(t *testing.T) {
		// Otherwise the account nobody ever used is the one the filter misses.
		page := listUsersWith(t, srv, "inactiveDays=1&q=invited@example.com")
		if !containsEmail(page, "invited@example.com") {
			t.Fatal("an account with no login must count as inactive")
		}
	})

	t.Run("who are the administrators", func(t *testing.T) {
		page := listUsersWith(t, srv, "role=ADMIN&q=boss@example.com")
		if !containsEmail(page, "boss@example.com") {
			t.Fatalf("role filter missed the administrator: %+v", page.Data.Items)
		}
		page = listUsersWith(t, srv, "role=USER&q=boss@example.com")
		if containsEmail(page, "boss@example.com") {
			t.Fatal("role filter is not filtering")
		}
	})

	t.Run("who still has a local password", func(t *testing.T) {
		page := listUsersWith(t, srv, "auth=LOCAL&q=arrived@example.com")
		if !containsEmail(page, "arrived@example.com") {
			t.Fatal("an account created here has a local password")
		}
		page = listUsersWith(t, srv, "auth=SSO&q=arrived@example.com")
		if containsEmail(page, "arrived@example.com") {
			t.Fatal("it is not an SSO account")
		}
	})
}

func TestAnInvalidFilterIsRefusedNotIgnored(t *testing.T) {
	// Silently ignoring it would show every user under a heading that says
	// the list is filtered.
	srv := newServerUnderTest(t)
	for _, query := range []string{"status=DELETED", "role=SUPERUSER", "auth=LDAP"} {
		resp, err := srv.admin.Get(srv.URL + "/api/v1/admin/users?" + query)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != 400 {
			t.Errorf("?%s = %d, want 400", query, resp.StatusCode)
		}
	}
}

func TestTheSortIsNeverTakenFromTheRequest(t *testing.T) {
	// An unknown value has to fall back, not reach the query.
	srv := newServerUnderTest(t)
	injection := url.QueryEscape("created_at; DROP TABLE users")
	page := listUsersWith(t, srv, "sort="+injection)
	if page.Data.Total == 0 {
		t.Fatal("the list should still answer")
	}
	var stillThere bool
	_ = srv.db.QueryRow(context.Background(),
		`SELECT EXISTS(SELECT 1 FROM users)`).Scan(&stillThere)
	if !stillThere {
		t.Fatal("the users table is gone")
	}
}

func containsEmail(page userPage, email string) bool {
	for _, item := range page.Data.Items {
		if item.Email == email {
			return true
		}
	}
	return false
}
