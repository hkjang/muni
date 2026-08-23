# muni

`muni`는 오프라인·폐쇄망 배포를 우선으로 설계한 Go + React 협업 문서 플랫폼입니다. Tiptap/ProseMirror 문서 모델, Yjs CRDT 공동편집, revision, 댓글·제안, 문서 ACL, 조건부 검토·승인, OpenAI 호환 스트리밍 AI, Keycloak OIDC, REST API와 MCP를 한 이미지로 제공합니다.

개인·팀 Workspace, 중첩 Folder, 최근/공유/즐겨찾기/휴지통, 팀 멤버, DOCX·Markdown·TXT·HTML Import, DOCX·PDF·HTML·Markdown·TXT Export, 첨부파일, PostgreSQL FTS, 감사 로그를 포함합니다. 개인 설정과 서비스 관리 영역은 라우트·권한·UI 수준에서 분리됩니다.

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

PDF Export는 이미지 안의 headless Chromium과 Noto CJK 글꼴을 사용하므로 외부 네트워크가 필요하지 않습니다. `/tmp`에 임시 렌더링 파일을 만들기 때문에 read-only 컨테이너에서는 제공된 tmpfs 설정을 유지하세요.

## Keycloak 설정

1. Keycloak에서 confidential OIDC client를 생성합니다.
2. Redirect URI에 `https://<muni-host>/api/v1/auth/oidc/callback`을 등록합니다.
3. muni의 **서비스 관리 → Keycloak OIDC**에서 realm issuer URL, client ID, client secret을 입력합니다.
4. Discovery 연결 테스트 후 OIDC를 활성화하고 저장합니다.

Issuer 예: `https://keycloak.internal/realms/company`. Endpoint는 discovery로 자동 결정하며 scope 기본값은 `openid profile email`입니다. 자동 프로비저닝과 기본 역할도 같은 화면에서 제어합니다.

## AI

관리자는 OpenAI-compatible base URL(`/v1`까지), API key, model, timeout과 max token을 설정합니다. 모든 사용자 호출은 `/api/v1/ai/chat`의 SSE stream으로 전달되고, max token은 시스템 상한 `262144`와 관리자 상한 중 작은 값으로 제한됩니다. 문서 Agent가 본문을 읽기 전에 사용자의 문서 ACL을 먼저 확인합니다.

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
