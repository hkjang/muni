package httpapi

import (
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestNotificationBodyNamesTheReaderAndSaysWhatHappened(t *testing.T) {
	id := uuid.New()
	body := notificationBody(pendingMail{
		displayName:  "홍길동",
		title:        "문서 검토 요청",
		body:         "검토 및 승인할 문서가 있습니다.",
		resourceType: "DOCUMENT",
		resourceID:   &id,
	}, "muni", "https://muni.example.com")

	if !strings.HasPrefix(body, "홍길동님,") {
		t.Fatalf("the mail should open with the reader's name: %q", body)
	}
	if !strings.Contains(body, "검토 및 승인할 문서가 있습니다.") {
		t.Fatalf("the mail should say what happened: %q", body)
	}
	if !strings.Contains(body, "https://muni.example.com/docs/"+id.String()) {
		t.Fatalf("the mail should link to the document: %q", body)
	}
	if !strings.Contains(body, "답장할 수 없습니다") {
		t.Fatalf("the mail should say it cannot be replied to: %q", body)
	}
}

func TestNotificationBodyWorksWithoutAServiceAddress(t *testing.T) {
	id := uuid.New()
	body := notificationBody(pendingMail{
		displayName: "홍길동", title: "알림", body: "무언가 일어났습니다.",
		resourceType: "DOCUMENT", resourceID: &id,
	}, "muni", "")
	if strings.Contains(body, "http") {
		t.Fatalf("with no service address there is nowhere to link: %q", body)
	}
	if !strings.Contains(body, "무언가 일어났습니다.") {
		t.Fatalf("the mail should still say what happened: %q", body)
	}
}

func TestNotificationLinkPointsAtTheRightScreen(t *testing.T) {
	id := uuid.New()
	if got := notificationLink("https://muni.example.com/", "DOCUMENT", &id); got != "https://muni.example.com/docs/"+id.String() {
		t.Fatalf("document link = %q", got)
	}
	if got := notificationLink("https://muni.example.com", "WORKSPACE", &id); got != "https://muni.example.com/workspace/"+id.String() {
		t.Fatalf("workspace link = %q", got)
	}
	// Something muni has no screen for still lands somewhere useful.
	if got := notificationLink("https://muni.example.com", "SETTINGS", &id); got != "https://muni.example.com" {
		t.Fatalf("fallback link = %q", got)
	}
	if got := notificationLink("https://muni.example.com", "DOCUMENT", nil); got != "" {
		t.Fatalf("a notification about nothing in particular has no link: %q", got)
	}
}

func TestNotificationBodyCarriesNoDocumentContent(t *testing.T) {
	// The body is the notification muni wrote, not the document. A mail that
	// carries content sends it to wherever the recipient forwards their mail.
	id := uuid.New()
	body := notificationBody(pendingMail{
		displayName: "홍길동", title: "제목", body: "검토할 문서가 있습니다.",
		resourceType: "DOCUMENT", resourceID: &id,
	}, "muni", "https://muni.example.com")
	if strings.Count(body, "\n\n") > 4 {
		t.Fatalf("the mail is longer than the notification it carries: %q", body)
	}
}
