# muni

`muni`는 오프라인·폐쇄망 배포를 우선으로 설계한 Go + React 협업 문서 플랫폼입니다. Tiptap/ProseMirror 문서 모델, Yjs CRDT 공동편집, revision, 댓글·제안, 문서 ACL, 조건부 검토·승인, OpenAI 호환 스트리밍 AI, Keycloak OIDC, REST API와 MCP를 한 이미지로 제공합니다.

개인·팀 Workspace, 중첩 Folder, 최근/공유/즐겨찾기/휴지통, 팀 멤버, PDF·DOCX·HWP·HWPX·Markdown·TXT·HTML Import, DOCX·PDF·HTML·Markdown·TXT Export, 첨부파일, PostgreSQL FTS, 감사 로그를 포함합니다. 개인 설정과 서비스 관리 영역은 라우트·권한·UI 수준에서 분리됩니다.

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

`ENCRYPTION_KEY`는 `openssl rand -base64 32`로 생성합니다. **이 키를 잃으면 봉인된 비밀값은 복구할 수 없습니다** — 데이터베이스 백업과 다른 곳에 보관하세요. 백업·복구·키 교체 절차는 [운영 안내](docs/OPERATIONS.md)에 있습니다. 최초 사용자가 생성된 뒤 bootstrap 값은 다시 계정을 덮어쓰지 않습니다. OIDC, AI, 워크플로, 보안, 공유와 Export 정책은 모두 **서비스 관리 → 서비스 설정**에서 변경합니다.

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

PDF Export는 이미지 안의 headless Chromium과 Noto CJK 글꼴을 사용하므로 외부 네트워크가 필요하지 않습니다. 렌더링마다 `/tmp` 아래에 임시 디렉터리를 만들고 Chromium의 `HOME`도 그 안으로 지정하기 때문에(비 root 계정의 홈이 없는 환경에서 Chromium이 기동하지 못하는 문제를 피합니다) read-only 컨테이너에서는 제공된 tmpfs 설정을 유지하세요. Chromium 실행 파일 경로는 `MUNI_CHROMIUM_PATH`로 바꿀 수 있습니다. Chromium 한 프로세스가 수백 MB를 쓰기 때문에 동시 실행 수를 `MUNI_PDF_CONCURRENCY`(기본 2)로 제한하며, 자리가 나기를 60초 넘게 기다리면 재시도 안내와 함께 거절합니다. 표는 페이지가 넘어가도 머리글 행이 반복되고 행 중간에서 잘리지 않습니다.

## 문서 Import·Export

Import와 Export는 모두 Tiptap/ProseMirror 문서 모델을 직접 다루므로 제목·목록·표·서식·이미지가 평문으로 무너지지 않습니다.

| 형식 | Import | Export | 유지되는 요소 |
| --- | --- | --- | --- |
| HWP | O | - | 한글 오피스의 옛 이진 형식입니다. OLE2 복합 파일 안에 압축된 이진 레코드가 들어 있어, Import 는 지금 **글자와 문단까지** 읽습니다. 서식·표·그림은 아직입니다. 암호가 걸린 문서는 무엇이 문제인지 말하고 거절합니다. |
| HWPX | O | - | 한글 오피스가 쓰는 XML 형식입니다. Import 는 제목 단계(개요 1~7), 문단 정렬·들여쓰기·줄간격, 굵게·기울임·밑줄·취소선·글자색·글꼴·크기, 표(가로·세로 병합, 머리글 행, 칸 음영·세로 정렬), 그림, 용지 방향을 읽습니다. |
| DOCX | O | O | 제목 단계, 글머리·번호·체크 목록(중첩 포함), 표(가로·세로 병합, 열 너비, 머리글 행, 칸 음영·세로 정렬), 굵게·기울임·밑줄·취소선·코드·형광·글자색·글꼴·크기·위첨자·아래첨자, 하이퍼링크, 문단 정렬·들여쓰기·줄간격, 인용, 코드 블록, 구분선, 쪽 나누기, 이미지, 각주·미주, 목차, 머리글·바닥글, 용지 방향 |
| PDF | O | O | Import는 텍스트 레이어에서 제목·문단·목록·표·코드 블록·이미지를 복원합니다. Export는 Chromium 렌더링으로 화면과 같은 레이아웃을 만듭니다. |
| Markdown | O | O | CommonMark에 GFM 표·체크 목록·취소선을 더해 양방향으로 처리합니다. 중첩 목록, 코드 펜스, 인용, 강조, 링크, data URI 이미지가 유지됩니다. |
| HTML | O | O | Export는 인쇄용 스타일시트와 base64로 인라인된 이미지를 포함합니다. Import는 표 병합·문단 정렬·인라인 스타일(색·글꼴·크기·형광)·체크 목록·중첩 목록까지 되살립니다. |
| Mermaid | - | O | `mermaid` 언어를 지정한 코드 블록은 편집기에서 도형으로 그려지고, HTML·PDF 로 내보낼 때도 그림으로 나갑니다. 다른 형식에서는 도형을 담을 수 없으므로 코드 그대로 남습니다 — 도형이 말하는 바가 곧 그 글이기 때문입니다. |
| TXT | O | O | 문단 구분과 줄바꿈이 유지되고, Export에서는 목록 기호와 표의 열 구분도 남습니다. |

Markdown·HTML Import는 muni가 내보낸 파일을 그대로 다시 읽어 들이도록 맞춰져 있습니다. `<u>`·`<mark>`처럼 CommonMark로 표현할 수 없어 HTML로 내보낸 서식, 표의 정렬 지정자, 체크박스 글리프도 원래 노드로 복원됩니다. 외부 URL 이미지는 서버가 대신 내려받지 않고(SSRF 방지) 설명 문구를 링크로 남깁니다.

DOCX Export는 `styles.xml`·`numbering.xml`·`theme1.xml`·`footnotes.xml`을 갖춘 정식 OOXML 패키지를 만듭니다. 바닥글에는 워드 자신의 `PAGE`·`NUMPAGES` 필드가 들어가므로 쪽 번호는 워드가 셉니다.

**워드 문서는 서식을 대개 그것이 붙은 자리가 아니라 스타일에 적어 둡니다.** Import는 그래서 문단 스타일의 들여쓰기·줄간격·정렬과 문자 스타일의 굵기·색을 `basedOn` 사슬을 따라 읽고, 그 위에 문단이나 런이 스스로 정한 것을 얹습니다 — 직접 준 쪽이 이깁니다. 표의 머리글 행도 마찬가지로 `w:tblLook`과 표 스타일의 첫 행 서식을 함께 보고 판단합니다(모든 행을 같게 그리는 Table Grid에는 머리글을 지어내지 않습니다). 목차는 워드가 마지막으로 계산해 둔 항목이 아니라 살아있는 목차 노드로 들어오고, 텍스트 상자 안의 글자와 미주도 함께 읽습니다. 그 밖에 스타일 이름(로케일 포함)·스타일에 붙은 numbering·`w:sym` 기호·hyperlink 관계·`w14:checkbox`를 해석하고, 확정되지 않은 삽입(`w:ins`)은 살리고 삭제(`w:del`)는 버립니다.

PDF Import는 xref가 손상된 파일도 객체 스캔으로 복구하고, Flate·LZW·ASCII85·ASCIIHex·RunLength 필터와 PNG predictor, 소유자 암호만 걸린 RC4/AES 암호화 문서를 지원합니다. 스캔본처럼 텍스트 레이어가 없는 PDF는 OCR 후 가져오라는 오류를 돌려줍니다.

`.hwp` 는 세 겹을 벗겨야 글자가 나옵니다 — OLE2 복합 파일, 스트림마다 걸린 raw deflate 압축(zlib 껍데기가 없어서 zlib 리더는 "invalid header" 를 냅니다), 그리고 레코드입니다. 문단 글자 안의 제어 표시는 **크기가 8 WCHAR, 즉 16바이트**입니다 — 형식 문서의 "8" 을 바이트로 읽으면 표시 한가운데에 내려앉아 그 뒷부분을 글자로 읽고, 문서에 없던 한자가 흩뿌려집니다.

**HWPX 는 서식을 문단이나 런에 적지 않고 머리(`Contents/header.xml`)에 적어 두고 번호로 가리킵니다.** 런에는 `charPrIDRef="7"` 하나만 있어서, 런만 읽으면 서식이 아예 없습니다 — muni 가 워드 스타일을 잃던 것과 같은 모양이라 이 가져오기는 머리부터 읽습니다. 표는 한글에서 문단 안에 들어 있고 muni 에서는 블록이므로 밖으로 꺼내며, 그림도 같습니다. 아직 Export 는 하지 않습니다.

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

편집기에서 문장을 드래그해 선택하면 선택 영역 위에 AI 메뉴가 떠서 다듬기·짧게·자세히·맞춤법·격식체·요약·목록으로·영어로 변환과 직접 지시를 실행할 수 있습니다. 결과는 문서에 바로 쓰이지 않고 미리보기로 먼저 보여 주며, **적용**을 눌러야 선택 영역을 대체합니다. 편집 모드에서 편집 권한이 있고 AI가 켜져 있을 때만 나타납니다.

문서 Agent는 필요하면 도구를 써서 답합니다. `tools: true`로 요청하면 모델이 문서 검색, 문서 읽기, 목차 확인, 버전 목록, 버전 비교를 스스로 호출한 뒤 답변을 만듭니다. 모든 도구는 **요청한 사용자의 문서 ACL을 그대로 적용**하므로 그 사람이 볼 수 없는 문서에는 닿지 않고, 호출 하나하나가 감사 로그에 남습니다. 도구는 읽기만 합니다 — 문서를 바꾸는 일은 사람이 받아들이는 제안으로 남깁니다. 편집기 Agent 패널에서 **워크스페이스 문서까지 찾아보기**를 켜면 사용할 수 있고, 어떤 도구를 거쳤는지 진행 중에 표시됩니다.

AI에게 문서 전체를 검토해 고칠 부분을 제안하도록 요청할 수도 있습니다. 결과는 문서에 바로 반영되지 않고 **동료가 남긴 것과 같은 변경 제안 목록**에 들어가며, 각 제안은 위치가 아니라 블록에 고정됩니다. 검토하는 동안 문서가 계속 바뀌어도 제안이 엉뚱한 곳을 가리키지 않고, 대상 블록이 사라졌다면 적용을 거부합니다.

AI가 답하기 전에 남기는 추론(`<think>` 구간)은 결과에서 제거합니다. 일부 모델은 추론을 켜지 않아도 여는 태그 없이 닫는 `</think>`만 내보내는데, 그대로 두면 선택 영역 기능이 그 추론을 문서에 그대로 써 넣습니다.

muni가 모델에 주는 지시(관리자 프롬프트, 대상 문서, 도구 사용법)는 **하나의 system 메시지로 합쳐서** 보냅니다. system 메시지를 하나만 허용하거나 맨 앞에만 허용하는 게이트웨이가 여러 개를 받으면 요청 자체를 거절하기 때문입니다.

게이트웨이마다 받아들이는 요청 형태가 달라서 400이 나는 일이 많기 때문에, 클라이언트는 거절 응답을 읽고 요청을 고쳐 다시 보냅니다. `max_tokens` → `max_completion_tokens` 전환, `stream_options`·`temperature` 제거, `system` → `developer`/`user` 역할 변경, 응답이 알려 준 토큰 상한 적용, 스트리밍 미지원 시 단일 응답을 SSE로 변환, base URL에 `/v1`이 빠진 경우 재시도를 자동으로 처리하고, 통과한 조합은 base URL·model 단위로 기억합니다. **서비스 관리 → 서비스 설정**의 AI 연결 테스트는 실제 호출 endpoint와 적용된 보정 내용을 함께 보여 줍니다.

## 빠른 이동

`Ctrl+K`(macOS는 `⌘K`), 또는 글을 쓰고 있지 않을 때 `/`를 누르면 빠른 이동 창이 열립니다. 최근 문서·워크스페이스·주요 화면이 바로 뜨고, 두 글자 이상 입력하면 서버 검색으로 본문까지 찾습니다. 방향키와 Enter로 이동합니다.

제목이 정확히 맞는 항목, 제목이 그 말로 시작하는 항목, 단어 첫머리가 맞는 항목 순으로 위에 옵니다. `추계`처럼 글자만 순서대로 친 경우도 `추진 계획`을 찾아 줍니다.

## 발표자료 만들기 (Ptium 연동)

문서를 발표자료로 만들 수 있습니다. muni는 발표자료 엔진을 직접 두지 않고 [Ptium](https://github.com/hkjang/ptium)에 REST로 요청합니다 — muni는 문서·협업·승인·버전을, Ptium은 스토리라인 설계·슬라이드 생성·디자인·Export를 맡습니다.

편집기의 내보내기 메뉴에서 **발표자료 만들기**를 고르면 대상·목적·발표 시간·상세 수준을 묻고, 그 값으로 슬라이드 수를 추천합니다. 40쪽짜리 문서를 40장으로 만들지 않기 위해서입니다 — 청중이 소화할 수 있는 분량은 원본 길이가 아니라 주어진 시간을 따릅니다.

문서를 평문으로 눌러 넘기지 않고 **ProseMirror 구조를 그대로 읽어** 중간 모델(Presentation Brief)로 옮깁니다. 번호 목록은 단계로, 날짜가 붙은 목록은 타임라인으로, 수치가 들어간 항목은 지표로, 표는 표로 전달되므로 Ptium이 KPI·프로세스·타임라인 컴포넌트를 고를 수 있습니다. 이 Brief는 muni 전용이 아니어서 나중에 다른 문서 소스도 같은 방식으로 붙일 수 있습니다.

만들어진 발표자료는 편집기 오른쪽 **발표자료** 탭에서 상태·슬라이드 수와 함께 보이고, Ptium 편집기로 바로 이동하거나 PPTX를 받을 수 있습니다. 발표자료를 만든 뒤 문서가 수정되면 **문서 변경됨**으로 표시됩니다.

**출처 표시**를 누르면 각 슬라이드에 근거가 된 문서와 버전, 해당 항목을 적어 넣습니다(`AI전략보고서 (Revision 23) | 추진 현황`). 숫자를 말하는 자료가 그 출처를 대지 못하면 회의에서 가장 먼저 나오는 질문이 그것입니다. 표지와 마무리 슬라이드는 주장을 담지 않으므로 건너뛰고, 이미 출처가 적힌 슬라이드는 손대지 않으며, 여러 번 눌러도 중복되지 않습니다.

발표자료를 만든 뒤 문서를 고쳤다면 **변경 반영**으로 어떤 슬라이드가 영향을 받는지 먼저 봅니다. 문서 diff를 블록 단위로 계산해 각 블록이 속한 제목을 찾고, 그 제목에 해당하는 슬라이드만 골라 `유지 / 다시 작성 / 추가 필요 / 삭제 후보`로 알려 줍니다. 실행하면 **바뀐 슬라이드만** 다시 작성하고 나머지는 원문 그대로 되돌려 놓기 때문에, Ptium 편집기에서 손본 레이아웃·문구·발표 메모가 살아남습니다. 전체를 다시 생성하면 그 작업이 전부 사라집니다.

muni는 연결 정보만 보관합니다. 슬라이드와 만들어진 파일은 Ptium에 있고, 사본을 두면 Ptium에서 편집하는 순간 두 시스템이 서로 다른 이야기를 하게 됩니다. Ptium 데이터베이스에 직접 접근하지 않고 REST만 사용하므로 Ptium의 데이터 모델이 바뀌어도 muni가 함께 깨지지 않습니다.

연결은 **서비스 관리 → 서비스 설정**에서 Ptium 주소와 API key로 설정하며, 저장 전에 연결 테스트를 할 수 있습니다.

## API와 MCP

- REST base: `/api/v1`
- OpenAPI: `/api/openapi.yaml`
- MCP Streamable HTTP: `/mcp`
- 인증: 브라우저 HttpOnly session 또는 개인 설정에서 1회 발급하는 `Authorization: Bearer muni_...` 키

API 키 scope는 `api:read`, `api:write`, `mcp:read`, `mcp:write`, `ai:use`입니다. MCP는 현행 `2026-07-28`의 무상태 header routing과 `server/discover`를 지원하고, `initialize`를 쓰는 2025 클라이언트에도 응답합니다. 쓰기 MCP 도구는 scope 확인 뒤 문서 ACL을 다시 검사합니다. 자세한 예는 [MCP 안내](docs/MCP.md)를 참고하세요.

## 개발과 검증

```bash
make test                     # placeholder 확인 + Go 테스트 + 프런트엔드 타입체크
make hooks                    # 같은 확인을 pre-commit 훅으로 설치(선택)
cd frontend && npm ci && npm run build
go run ./cmd/muni
```

`webui/dist`에는 프런트엔드를 빌드하지 않은 체크아웃에서도 `go build`가 되도록 placeholder `index.html`만 커밋되어 있습니다. `npm run build`가 이 파일을 덮어쓰는데 옆에 생기는 asset은 gitignore 대상이라, 빌드된 `index.html`을 커밋하면 저장소에 없는 파일을 가리키게 됩니다. `scripts/check-webui-placeholder.sh`가 CI와 `make test`에서 이를 막아 줍니다.

업로드 파서(PDF·DOCX·Markdown·HTML)에는 퍼즈 타깃이 있습니다.

```bash
go test ./internal/pdfx -run FuzzImport -fuzz FuzzImport -fuzztime=60s
```

버전 기록 패널에서 두 버전을 선택하면 블록 단위로 무엇이 바뀌었는지 비교합니다. 블록마다 붙은 안정적인 ID로 짝을 맞추기 때문에 위에 문단을 하나 넣었다고 그 아래가 전부 바뀐 것으로 보이지 않고, 추가·삭제·변경·이동이 구분됩니다. 변경된 블록은 단어 단위(한글은 글자 단위)로 어디가 달라졌는지 표시합니다. 같은 비교 결과는 `GET /api/v1/documents/{id}/revisions/{from}/diff/{to}`로도 받을 수 있습니다.

공동편집 이력은 문서마다 하나의 병합 상태(`collab_snapshots`)로 압축됩니다. 누적된 update가 임계값(400건 또는 4MiB)을 넘으면 서버가 접속 중인 편집자 한 명에게 압축을 요청하고, 그 클라이언트가 되돌려 준 전체 상태로 이전 update를 대체합니다. 문서를 열 때 내려받는 양이 이력 길이에 비례해 늘어나지 않습니다.

실제 PostgreSQL과 브라우저가 준비된 개발 환경에서는 `cd frontend && npm run test:e2e`로 로그인·문서 저장·새로고침 라우트 복원·관리 화면·두 세션 실시간 동기화를 검증합니다.

Docker가 설치된 환경에서는 다음으로 릴리스와 같은 이미지를 만듭니다.

```bash
./scripts/package-offline.sh v$(cat VERSION)
```

Git tag `vMAJOR.MINOR.PATCH`를 `https://github.com/hkjang/muni`에 push하면 GitHub Actions가 `muni:v버전` 이미지를 만들고 `muni-v버전.tar.gz` 하나를 Release asset으로 게시합니다.

## 주요 보안 성질

- Argon2id 비밀번호 hash, SHA-256 session/API token hash, HttpOnly/SameSite cookie
- AES-256-GCM과 associated data를 이용한 OIDC/AI secret 및 사용자 data key envelope encryption
- OWNER/EDITOR/COMMENTER/VIEWER 문서 ACL을 조회·검색·AI·MCP·공동편집에 일관 적용
- 공개 링크, 승인 흐름, PDF/DOCX, API key 수명과 읽기 감사 로그를 관리자 정책으로 제어
- 비 root, read-only root filesystem, 모든 Linux capability drop 배포 예제

전체 구조와 운영 경계는 [아키텍처 문서](docs/ARCHITECTURE.md)에 정리되어 있습니다.
