package httpapi

import (
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/mail"
	"strings"

	"github.com/google/uuid"
	"github.com/hkjang/muni/internal/database"
	"github.com/hkjang/muni/internal/mailer"
	"github.com/jackc/pgx/v5"
)

// createUser lets an administrator make an account.
//
// There was no way to. Accounts appeared in exactly two places: the container
// creating one administrator at first boot, and OIDC provisioning somebody on
// their first sign-in. An office running muni without OIDC — which is the
// offline installation muni is built for — had one account and no way to make
// a second. The setting called allowLocalLogin was true and meant nothing,
// because no local account could ever exist.
func (s *Server) createUser(w http.ResponseWriter, r *http.Request) {
	p, _ := principalFrom(r.Context())
	var input struct {
		Email       string `json:"email"`
		Username    string `json:"username"`
		DisplayName string `json:"displayName"`
		Role        string `json:"role"`
		Locale      string `json:"locale"`
		// Password is optional. Left out, muni chooses one and returns it
		// once — an administrator inventing passwords for forty people
		// invents forty variations of the same one.
		Password string `json:"password"`
		// SendEmail asks muni to mail the credentials. It needs SMTP to be
		// configured; without it the request still creates the account and
		// says the mail did not go.
		SendEmail bool `json:"sendEmail"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}

	email := strings.ToLower(strings.TrimSpace(input.Email))
	if _, err := mail.ParseAddress(email); err != nil || strings.Count(email, "@") != 1 {
		writeError(w, 400, "INVALID_EMAIL", "이메일 주소를 확인해 주세요.")
		return
	}
	if input.Role != "" && !contains([]string{"ADMIN", "USER"}, input.Role) {
		writeError(w, 400, "INVALID_ROLE", "역할 값이 올바르지 않습니다.")
		return
	}
	if name := strings.TrimSpace(input.DisplayName); len([]rune(name)) > 100 {
		writeError(w, 400, "INVALID_DISPLAY_NAME", "표시 이름을 확인해 주세요.")
		return
	}

	// A password the administrator typed is theirs to justify; one muni
	// generated still has to be replaced, because the administrator saw it.
	password := input.Password
	generated := password == ""
	if generated {
		var err error
		if password, err = generatePassword(); err != nil {
			writeError(w, 500, "PASSWORD_GENERATION_FAILED", "임시 비밀번호를 만들지 못했습니다.")
			return
		}
	} else if err := checkPassword(password); err != nil {
		writeError(w, 400, "WEAK_PASSWORD", err.Error())
		return
	}

	hash, err := database.HashPassword(password)
	if err != nil {
		writeError(w, 500, "PASSWORD_HASH_FAILED", "비밀번호를 저장하지 못했습니다.")
		return
	}

	var created User
	err = database.WithTx(r.Context(), s.db, func(tx pgx.Tx) error {
		var taken bool
		if err := tx.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM users WHERE email=$1)`, email).Scan(&taken); err != nil {
			return err
		}
		if taken {
			return errEmailTaken
		}
		created, err = provisionUser(r.Context(), tx, s.sealer, provisionSpec{
			Username:           input.Username,
			Email:              email,
			DisplayName:        input.DisplayName,
			Role:               input.Role,
			Locale:             input.Locale,
			PasswordHash:       &hash,
			MustChangePassword: true,
			CreatedBy:          &p.User.ID,
		})
		return err
	})
	if errors.Is(err, errEmailTaken) {
		writeError(w, 409, "EMAIL_TAKEN", "이미 그 이메일을 쓰는 계정이 있습니다.")
		return
	}
	if err != nil {
		s.logger.Error("account provisioning failed", "error", err, "email", email)
		writeError(w, 500, "DATABASE_ERROR", "계정을 만들지 못했습니다.")
		return
	}

	s.audit(r, &p.User.ID, "CREATE_USER", "USER", &created.ID,
		map[string]any{"email": email, "role": created.Role, "generatedPassword": generated})

	response := map[string]any{
		"id": created.ID, "username": created.Username, "email": created.Email,
		"displayName": created.DisplayName, "role": created.Role, "status": created.Status,
		"passwordResetRequired": true,
	}
	// The generated password is shown exactly here. It is not stored in
	// readable form and no later request can produce it again; an
	// administrator who loses it sets a new one.
	if generated {
		response["temporaryPassword"] = password
	}
	if input.SendEmail {
		sent, reason := s.mailCredentials(r.Context(), created, password)
		response["emailSent"] = sent
		if !sent {
			response["emailError"] = reason
		}
	}
	writeData(w, 201, response)
}

var errEmailTaken = errors.New("email already registered")

// mailCredentials sends the new account its sign-in details. A failure here
// does not undo the account: the administrator can read the password off the
// response and pass it on themselves.
func (s *Server) mailCredentials(ctx context.Context, user User, password string) (bool, string) {
	all, err := s.settings.GetAll(ctx, true)
	if err != nil {
		return false, "설정을 읽지 못했습니다."
	}
	if !all.SMTP.Enabled {
		return false, "메일 발송이 꺼져 있습니다."
	}
	sender := mailerFor(all)
	if !sender.Usable() {
		return false, "메일 서버가 설정되어 있지 않습니다."
	}
	service := strings.TrimSpace(all.General.ServiceName)
	if service == "" {
		service = "muni"
	}
	web := strings.TrimRight(strings.TrimSpace(all.SMTP.BaseURL), "/")
	if web == "" {
		web = "(관리자에게 주소를 문의하세요)"
	}
	body := fmt.Sprintf(`%s님, %s 계정이 만들어졌습니다.

  주소: %s
  아이디: %s
  임시 비밀번호: %s

처음 로그인하면 비밀번호를 바꾸도록 안내합니다. 바꾸신 뒤에는 이 메일을 지워 주세요.`,
		user.DisplayName, service, web, user.Username, password)
	if err := sender.Send(mailer.Message{
		To:      user.Email,
		Subject: service + " 계정이 만들어졌습니다",
		Body:    body,
	}); err != nil {
		return false, err.Error()
	}
	return true, ""
}

// importUsers creates accounts from a CSV, because onboarding is something
// that happens to a department at once rather than to one person.
//
// Every row is reported: the ones that worked with the password to hand out,
// the ones that did not with the reason. Refusing all forty because the
// thirty-first has a typo helps nobody, and skipping it quietly is worse.
func (s *Server) importUsers(w http.ResponseWriter, r *http.Request) {
	p, _ := principalFrom(r.Context())
	if err := r.ParseMultipartForm(2 << 20); err != nil {
		writeError(w, 400, "INVALID_UPLOAD", "파일을 읽지 못했습니다.")
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, 400, "FILE_REQUIRED", "CSV 파일을 올려 주세요.")
		return
	}
	defer file.Close()
	if header.Size > 2<<20 {
		writeError(w, 413, "FILE_TOO_LARGE", "CSV는 2MB 이하여야 합니다.")
		return
	}

	rows, err := readUserCSV(file)
	if err != nil {
		writeError(w, 400, "INVALID_CSV", err.Error())
		return
	}
	if len(rows) == 0 {
		writeError(w, 400, "EMPTY_CSV", "가져올 행이 없습니다.")
		return
	}
	if len(rows) > 500 {
		writeError(w, 400, "TOO_MANY_ROWS", "한 번에 500행까지 가져올 수 있습니다.")
		return
	}

	results := make([]map[string]any, 0, len(rows))
	var succeeded int
	for _, row := range rows {
		result := map[string]any{"line": row.Line, "email": row.Email}
		user, password, err := s.provisionFromRow(r, p.User.ID, row)
		if err != nil {
			result["ok"] = false
			result["error"] = err.Error()
			results = append(results, result)
			continue
		}
		succeeded++
		result["ok"] = true
		result["id"] = user.ID
		result["username"] = user.Username
		result["temporaryPassword"] = password
		results = append(results, result)
	}
	s.audit(r, &p.User.ID, "IMPORT_USERS", "USER", nil,
		map[string]any{"rows": len(rows), "created": succeeded, "failed": len(rows) - succeeded})
	writeData(w, 200, map[string]any{"created": succeeded, "failed": len(rows) - succeeded, "results": results})
}

func (s *Server) provisionFromRow(r *http.Request, actor uuid.UUID, row userRow) (User, string, error) {
	password, err := generatePassword()
	if err != nil {
		return User{}, "", errors.New("임시 비밀번호를 만들지 못했습니다")
	}
	hash, err := database.HashPassword(password)
	if err != nil {
		return User{}, "", errors.New("비밀번호를 저장하지 못했습니다")
	}
	var created User
	err = database.WithTx(r.Context(), s.db, func(tx pgx.Tx) error {
		var taken bool
		if err := tx.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM users WHERE email=$1)`, row.Email).Scan(&taken); err != nil {
			return err
		}
		if taken {
			return errEmailTaken
		}
		created, err = provisionUser(r.Context(), tx, s.sealer, provisionSpec{
			Username: row.Username, Email: row.Email, DisplayName: row.DisplayName,
			Role: row.Role, PasswordHash: &hash, MustChangePassword: true, CreatedBy: &actor,
		})
		return err
	})
	if errors.Is(err, errEmailTaken) {
		return User{}, "", errors.New("이미 그 이메일을 쓰는 계정이 있습니다")
	}
	if err != nil {
		return User{}, "", errors.New("계정을 만들지 못했습니다")
	}
	return created, password, nil
}

type userRow struct {
	Line        int
	Email       string
	Username    string
	DisplayName string
	Role        string
}

// readUserCSV accepts the file a person exported from their HR system: a
// header naming the columns in any order, and the columns they did not fill
// left out entirely. Only the email is required — everything else has a
// sensible value derived from it.
func readUserCSV(source io.Reader) ([]userRow, error) {
	reader := csv.NewReader(source)
	reader.FieldsPerRecord = -1
	reader.TrimLeadingSpace = true

	header, err := reader.Read()
	if err != nil {
		return nil, errors.New("CSV의 첫 줄을 읽지 못했습니다")
	}
	// A file saved from Excel on Windows starts with a byte order mark, which
	// would otherwise make the first column name unmatchable.
	if len(header) > 0 {
		header[0] = strings.TrimPrefix(header[0], "\ufeff")
	}
	index := map[string]int{}
	for i, name := range header {
		index[strings.ToLower(strings.TrimSpace(name))] = i
	}
	column := func(record []string, names ...string) string {
		for _, name := range names {
			if i, ok := index[name]; ok && i < len(record) {
				return strings.TrimSpace(record[i])
			}
		}
		return ""
	}
	if _, ok := index["email"]; !ok {
		if _, ok := index["이메일"]; !ok {
			return nil, errors.New("email 열이 필요합니다 (첫 줄에 열 이름을 넣어 주세요)")
		}
	}

	var rows []userRow
	line := 1
	for {
		record, err := reader.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		line++
		if err != nil {
			return nil, fmt.Errorf("%d번째 줄을 읽지 못했습니다", line)
		}
		email := strings.ToLower(column(record, "email", "이메일"))
		if email == "" {
			continue // A blank line at the end of a spreadsheet is not an error.
		}
		role := strings.ToUpper(column(record, "role", "역할"))
		if role != "ADMIN" {
			role = "USER"
		}
		rows = append(rows, userRow{
			Line:        line,
			Email:       email,
			Username:    column(record, "username", "아이디"),
			DisplayName: column(record, "displayname", "display_name", "name", "이름", "표시이름"),
			Role:        role,
		})
	}
	return rows, nil
}
