package httpapi

import (
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/hkjang/muni/internal/settings"
)

func settingsAllMailNotifications() settings.MailNotify { return settings.AllMailNotifications() }

func TestNotificationBodyNamesTheReaderAndSaysWhatHappened(t *testing.T) {
	id := uuid.New()
	body := notificationBody([]pendingMail{{
		displayName:  "홍길동",
		title:        "문서 검토 요청",
		body:         "검토 및 승인할 문서가 있습니다.",
		resourceType: "DOCUMENT",
		resourceID:   &id,
	}}, "muni", "https://muni.example.com")

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
	body := notificationBody([]pendingMail{{
		displayName: "홍길동", title: "알림", body: "무언가 일어났습니다.",
		resourceType: "DOCUMENT", resourceID: &id,
	}}, "muni", "")
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
	body := notificationBody([]pendingMail{{
		displayName: "홍길동", title: "제목", body: "검토할 문서가 있습니다.",
		resourceType: "DOCUMENT", resourceID: &id,
	}}, "muni", "https://muni.example.com")
	if strings.Count(body, "\n\n") > 4 {
		t.Fatalf("the mail is longer than the notification it carries: %q", body)
	}
}

func TestBundleByRecipientMakesOneMailPerPerson(t *testing.T) {
	// A comment that mentions somebody twice, or two approvals landing in
	// the same minute, is one mail — a burst is how people learn to filter
	// muni out, and the filter takes the approvals with it.
	hong, kim := uuid.New(), uuid.New()
	pending := []pendingMail{
		{id: uuid.New(), userID: hong, email: "hong@example.com", title: "첫째"},
		{id: uuid.New(), userID: kim, email: "kim@example.com", title: "둘째"},
		{id: uuid.New(), userID: hong, email: "hong@example.com", title: "셋째"},
	}
	bundles := bundleByRecipient(pending)
	if len(bundles) != 2 {
		t.Fatalf("two people should get two mails, got %d", len(bundles))
	}
	if len(bundles[0]) != 2 || bundles[0][0].title != "첫째" || bundles[0][1].title != "셋째" {
		t.Fatalf("hong's bundle should hold both in order: %+v", bundles[0])
	}
	if len(bundles[1]) != 1 || bundles[1][0].email != "kim@example.com" {
		t.Fatalf("kim's bundle: %+v", bundles[1])
	}
}

func TestBundledMailListsEachNotificationAndCountsThemInTheSubject(t *testing.T) {
	docA, docB := uuid.New(), uuid.New()
	bundle := []pendingMail{
		{userID: uuid.New(), displayName: "홍길동", kind: "APPROVAL_REQUEST", title: "문서 검토 요청", body: "검토 및 승인할 문서가 있습니다.", resourceType: "DOCUMENT", resourceID: &docA},
		{userID: uuid.New(), displayName: "홍길동", kind: "MENTION", title: "문서 댓글에서 회원님을 멘션했습니다.", body: "@hong 확인 부탁", resourceType: "DOCUMENT", resourceID: &docB},
	}
	if got := mailSubject(bundle, "muni"); got != "[muni] 문서 검토 요청 외 1건" {
		t.Fatalf("subject = %q", got)
	}
	if got := mailSubject(bundle[:1], "muni"); got != "문서 검토 요청" {
		t.Fatalf("a single notification keeps its own title: %q", got)
	}
	body := notificationBody(bundle, "muni", "https://muni.example.com")
	for _, want := range []string{"1. 문서 검토 요청", "2. 문서 댓글에서 회원님을 멘션했습니다.",
		"https://muni.example.com/docs/" + docA.String(), "https://muni.example.com/docs/" + docB.String()} {
		if !strings.Contains(body, want) {
			t.Fatalf("the bundle should list %q: %q", want, body)
		}
	}
	if strings.Count(body, "홍길동님,") != 1 {
		t.Fatalf("the reader is greeted once: %q", body)
	}
	if got := mailEventOf(bundle); got != "DIGEST" {
		t.Fatalf("a mixed bundle is recorded as a digest, got %q", got)
	}
	if got := mailEventOf(bundle[:1]); got != "APPROVAL_REQUEST" {
		t.Fatalf("a single notification is recorded by its type, got %q", got)
	}
}

func TestEventSwitchesSelectWhatIsMailed(t *testing.T) {
	// Every switch is on by default, so a fresh install mails all four.
	all := mailedNotificationTypes(settingsAllMailNotifications())
	if len(all) != 4 {
		t.Fatalf("expected four events by default, got %v", all)
	}
	notify := settingsAllMailNotifications()
	notify.Mention = false
	types := mailedNotificationTypes(notify)
	if len(types) != 3 {
		t.Fatalf("one switch off should drop one type, got %v", types)
	}
	for _, kind := range types {
		if kind == "MENTION" {
			t.Fatalf("mentions are switched off yet still mailed: %v", types)
		}
	}
}

func TestAPIKeyNotificationsLinkToPersonalSettings(t *testing.T) {
	id := uuid.New()
	if got := notificationLink("https://muni.example.com", "API_KEY", &id); got != "https://muni.example.com/settings" {
		t.Fatalf("an expiring key is managed in personal settings: %q", got)
	}
}

func TestHumanBytesReadsLikeAnOperatorWouldSayIt(t *testing.T) {
	cases := map[int64]string{
		0:       "0 B",
		900:     "900 B",
		1536:    "1.5 KB",
		5 << 20: "5.0 MB",
		3 << 30: "3.0 GB",
		2 << 40: "2.0 TB",
	}
	for input, want := range cases {
		if got := humanBytes(input); got != want {
			t.Fatalf("humanBytes(%d) = %q, want %q", input, got, want)
		}
	}
}

func TestHumanBytesHandlesNonsense(t *testing.T) {
	if got := humanBytes(-1); got != "0 B" {
		t.Fatalf("a negative size is not a size: %q", got)
	}
}
