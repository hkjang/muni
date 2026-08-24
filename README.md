# muni

`muni`는 오프라인·폐쇄망 배포를 우선으로 설계한 Go + React 협업 문서 플랫폼입니다. Tiptap/ProseMirror 문서 모델, Yjs CRDT 공동편집, revision, 댓글·제안, 문서 ACL, 조건부 검토·승인, OpenAI 호환 스트리밍 AI, Keycloak OIDC, REST API와 MCP를 한 이미지로 제공합니다.

개인·팀 Workspace, 중첩 Folder, 최근/공유/즐겨찾기/휴지통, 팀 멤버, PDF·DOCX·Markdown·TXT·HTML Import, DOCX·PDF·HTML·Markdown·TXT Export, 첨부파일, PostgreSQL FTS, 감사 로그를 포함합니다. 개인 설정과 서비스 관리 영역은 라우트·권한·UI 수준에서 분리됩니다.

## 기술 선택

- UI: React 19 + Material UI 7. MUI는 관리자 폼과 접근성 기본기가 좋고, 테마를 통해 본문 16px·편집기 17px 이상의 가독성을 일관되게 유지합니다.
- Editor: Tiptap 3 + ProseMirror JSON. 문서를 HTML 문자열 하나로 저장하지 않습니다.
- Collaboration: Yjs + WebSocket + IndexedDB. 연결이 끊겨도 로컬 CRDT 편집을 유지하고 재접속 시 병합합니다.
- Backend: Go 1.27 modular monolith. React 번들을 바이너리에 embed합니다.
- Storage/Search: PostgreSQL 15+의 JSONB와 FTS. 런타임에 별도 Redis, MinIO, 검색 엔진이 필요하지 않습니다.

## 런타임 환경변수

애플리케이션은 정확히 아래 네 값만 환경변수로 받습니다.

| 변수 | 용도 |
| --- | --- |
| `POSTGRES_DSN` | PostgreSQL 접속 문자열 |
| `BOOTSTRAP_ADMIN` | 최초 관리자 아이디 또는 이메일 |
| `BOOTSTRAP_ADMIN_PASSWORD` | 최초 관리자 비밀번호(12자 이상) |
| `ENCRYPTION_KEY` | 설정 secret과 사용자 data key를 봉인하는 base64 32-byte master key |

필수는 위 네 개뿐이고, 아래 두 개는 PDF 변환 동작을 조정할 때만 씁니다.

| 선택 변수 | 기본값 | 용도 |
| --- | --- | --- |
| `MUNI_CHROMIUM_PATH` | 자동 탐색 | PDF Export에 쓸 headless 브라우저 실행 파일 경로 |
| `MUNI_PDF_CONCURRENCY` | `2` | 동시에 띄울 Chromium 프로세스 수(1~32) |

`ENCRYPTION_KEY`는 `openssl rand -base64 32`로 생성합니다. 최초 사용자가 생성된 뒤 bootstrap 값은 다시 계정을 덮어쓰지 않습니다. OIDC, AI, 워크플로, 보안, 공유와 Export 정책은 모두 **서비스 관리 → 서비스 설정**에서 변경합니다.

## 오프라인 이미지 실행

릴리스 파일명과 포함 이미지 태그는 다음 규칙을 지킵니다.

```text
asset: muni-v0.1.0.tar.gz
image: muni:v0.1.0
```

```bash
gzip -dc muni-v0.1.0.tar.gz | docker load
cp .env.example .env
# .env의 네 값을 안전한 값으로 변경
docker compose -f compose.example.yaml --env-file .env up -d
```

PostgreSQL은 미리 준비되어 있어야 하며 DSN 계정에는 최초 실행 시 schema와 `pgcrypto`, `citext` extension을 만들 권한이 필요합니다. 서비스는 고정 포트 `8080`을 사용합니다. TLS는 사내 ingress/reverse proxy에서 종료하는 구성을 권장합니다.

PDF Export는 이미지 안의 headless Chromium과 Noto CJK 글꼴을 사용하므로 외부 네트워크가 필요하지 않습니다. `/tmp`에 임시 렌더링 파일을 만들기 때문에 read-only 컨테이너에서는 제공된 tmpfs 설정을 유지하세요. Chromium 실행 파일 경로는 `MUNI_CHROMIUM_PATH`로 바꿀 수 있습니다. Chromium 한 프로세스가 수백 MB를 쓰기 때문에 동시 실행 수를 `MUNI_PDF_CONCURRENCY`(기본 2)로 제한하며, 자리가 나기를 60초 넘게 기다리면 재시도 안내와 함께 거절합니다. 표는 페이지가 넘어가도 머리글 행이 반복되고 행 중간에서 잘리지 않습니다.

## 문서 Import·Export

Import와 Export는 모두 Tiptap/ProseMirror 문서 모델을 직접 다루므로 제목·목록·표·서식·이미지가 평문으로 무너지지 않습니다.

| 형식 | Import | Export | 유지되는 요소 |
| --- | --- | --- | --- |
| DOCX | O | O | 제목 단계, 글머리·번호·체크 목록(중첩 포함), 표(가로·세로 병합, 열 너비, 머리글 행), 굵게·기울임·밑줄·취소선·코드·형광·글자색·글꼴·크기, 하이퍼링크, 문단 정렬, 인용, 코드 블록, 구분선, 이미지 |
| PDF | O | O | Import는 텍스트 레이어에서 제목·문단·목록·표·코드 블록·이미지를 복원합니다. Export는 Chromium 렌더링으로 화면과 같은 레이아웃을 만듭니다. |
| Markdown | O | O | CommonMark에 GFM 표·체크 목록·취소선을 더해 양방향으로 처리합니다. 중첩 목록, 코드 펜스, 인용, 강조, 링크, data URI 이미지가 유지됩니다. |
| HTML | O | O | Export는 인쇄용 스타일시트와 base64로 인라인된 이미지를 포함합니다. Import는 표 병합·문단 정렬·인라인 스타일(색·글꼴·크기·형광)·체크 목록·중첩 목록까지 되살립니다. |
| TXT | O | O | 문단 구분과 줄바꿈이 유지되고, Export에서는 목록 기호와 표의 열 구분도 남습니다. |

Markdown·HTML Import는 muni가 내보낸 파일을 그대로 다시 읽어 들이도록 맞춰져 있습니다. `<u>`·`<mark>`처럼 CommonMark로 표현할 수 없어 HTML로 내보낸 서식, 표의 정렬 지정자, 체크박스 글리프도 원래 노드로 복원됩니다. 외부 URL 이미지는 서버가 대신 내려받지 않고(SSRF 방지) 설명 문구를 링크로 남깁니다.

DOCX Export는 `styles.xml`·`numbering.xml`·`theme1.xml`을 갖춘 정식 OOXML 패키지를 만들고, Import는 스타일 이름(로케일 포함)·스타일에 붙은 numbering·`w:sym` 기호·hyperlink 관계·`w14:checkbox`를 함께 해석합니다. PDF Import는 xref가 손상된 파일도 객체 스캔으로 복구하고, Flate·LZW·ASCII85·ASCIIHex·RunLength 필터와 PNG predictor, 소유자 암호만 걸린 RC4/AES 암호화 문서를 지원합니다. 스캔본처럼 텍스트 레이어가 없는 PDF는 OCR 후 가져오라는 오류를 돌려줍니다.

Import한 이미지는 문서 첨부파일로 저장되며, 같은 그림은 내용 해시로 한 번만 저장합니다. 페이지 절반 이상에 반복해서 나타나는 그림은 레터헤드 로고나 워터마크로 보고 본문에서 제외합니다. PDF 해석은 CPU를 쓰는 작업이라 업로드 한 건당 90초 제한과 페이지·텍스트 조각 수 상한을 두어, 조작된 파일이 워커를 붙잡지 못하게 합니다.

업로드 파서(PDF·DOCX·Markdown·HTML)는 신뢰할 수 없는 입력을 다루므로 `go test -fuzz`용 퍼즈 타깃을 함께 두었습니다.

## Keycloak 설정

1. Keycloak에서 confidential OIDC client를 생성합니다.
2. Redirect URI에 `https://<muni-host>/api/v1/auth/oidc/callback`을 등록합니다.
3. muni의 **서비스 관리 → Keycloak OIDC**에서 realm issuer URL, client ID, client secret을 입력합니다.
4. Discovery 연결 테스트 후 OIDC를 활성화하고 저장합니다.

Issuer 예: `https://keycloak.internal/realms/company`. Endpoint는 discovery로 자동 결정하며 scope 기본값은 `openid profile email`입니다. 자동 프로비저닝과 기본 역할도 같은 화면에서 제어합니다.

## AI

관리자는 OpenAI-compatible base URL(`/v1`까지), API key, model, timeout과 max token을 설정합니다. 모든 사용자 호출은 `/api/v1/ai/chat`의 SSE stream으로 전달되고, max token은 시스템 상한 `262144`와 관리자 상한 중 작은 값으로 자동 조정됩니다(초과 요청은 거절 대신 상한으로 맞춥니다). 문서 Agent가 본문을 읽기 전에 사용자의 문서 ACL을 먼저 확인합니다.

게이트웨이마다 받아들이는 요청 형태가 달라서 400이 나는 일이 많기 때문에, 클라이언트는 거절 응답을 읽고 요청을 고쳐 다시 보냅니다. `max_tokens` → `max_completion_tokens` 전환, `stream_options`·`temperature` 제거, `system` → `developer`/`user` 역할 변경, 응답이 알려 준 토큰 상한 적용, 스트리밍 미지원 시 단일 응답을 SSE로 변환, base URL에 `/v1`이 빠진 경우 재시도를 자동으로 처리하고, 통과한 조합은 base URL·model 단위로 기억합니다. **서비스 관리 → 서비스 설정**의 AI 연결 테스트는 실제 호출 endpoint와 적용된 보정 내용을 함께 보여 줍니다.

## API와 MCP

- REST base: `/api/v1`
- OpenAPI: `/api/openapi.yaml`
- MCP Streamable HTTP: `/mcp`
- 인증: 브라우저 HttpOnly session 또는 개인 설정에서 1회 발급하는 `Authorization: Bearer muni_...` 키

API 키 scope는 `api:read`, `api:write`, `mcp:read`, `mcp:write`, `ai:use`입니다. MCP는 현행 `2026-07-28`의 무상태 header routing과 `server/discover`를 지원하고, `initialize`를 쓰는 2025 클라이언트에도 응답합니다. 쓰기 MCP 도구는 scope 확인 뒤 문서 ACL을 다시 검사합니다. 자세한 예는 [MCP 안내](docs/MCP.md)를 참고하세요.

## 개발과 검증

```bash
cd frontend && npm ci && npm run build
go test ./...
go run ./cmd/muni
```

실제 PostgreSQL과 브라우저가 준비된 개발 환경에서는 `cd frontend && npm run test:e2e`로 로그인·문서 저장·새로고침 라우트 복원·관리 화면·두 세션 실시간 동기화를 검증합니다.

Docker가 설치된 환경에서는 다음으로 릴리스와 같은 이미지를 만듭니다.

```bash
./scripts/package-offline.sh v0.1.0
```

Git tag `vMAJOR.MINOR.PATCH`를 `https://github.com/hkjang/muni`에 push하면 GitHub Actions가 `muni:v버전` 이미지를 만들고 `muni-v버전.tar.gz` 하나를 Release asset으로 게시합니다.

## 주요 보안 성질

- Argon2id 비밀번호 hash, SHA-256 session/API token hash, HttpOnly/SameSite cookie
- AES-256-GCM과 associated data를 이용한 OIDC/AI secret 및 사용자 data key envelope encryption
- OWNER/EDITOR/COMMENTER/VIEWER 문서 ACL을 조회·검색·AI·MCP·공동편집에 일관 적용
- 공개 링크, 승인 흐름, PDF/DOCX, API key 수명과 읽기 감사 로그를 관리자 정책으로 제어
- 비 root, read-only root filesystem, 모든 Linux capability drop 배포 예제

전체 구조와 운영 경계는 [아키텍처 문서](docs/ARCHITECTURE.md)에 정리되어 있습니다.
