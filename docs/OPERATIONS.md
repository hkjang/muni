# 운영 안내

백업, 복구, 마스터 키 관리. 사고가 난 뒤에 읽으면 늦는 문서입니다.

## 무엇을 지켜야 하는가

muni의 상태는 **두 가지**뿐입니다.

| | 무엇 | 잃으면 |
| --- | --- | --- |
| 데이터베이스 | 문서, 버전, 댓글, 첨부, 설정, 감사 로그 | 전부 |
| `ENCRYPTION_KEY` | 비밀값을 봉인한 master key | 아래 참조 |

이미지에는 상태가 없습니다. 컨테이너는 언제든 버리고 다시 만들어도 됩니다.

**첨부 파일도 데이터베이스 안에 있습니다.** 별도로 백업할 파일 디렉터리가 없는 대신, 데이터베이스 백업이 첨부까지 포함하며 그만큼 커집니다.

## ENCRYPTION_KEY를 잃으면

이 키는 다음을 봉인합니다.

- 서비스 설정의 비밀값 — AI API key, OIDC client secret, SMTP 비밀번호, Ptium API key
- 사용자별 data key (`user_keys.wrapped_key`)

**복구 수단은 없습니다.** 봉인된 값은 이 키로만 열립니다. 백업된 데이터베이스가 있어도, 키가 없으면 그 안의 비밀값은 열 수 없습니다.

키를 잃었을 때 할 수 있는 일은 **다시 설정하는 것**뿐입니다.

1. 새 키를 만들어 `ENCRYPTION_KEY`에 넣고 기동합니다.
2. `서비스 관리 → 서비스 설정`에서 AI, OIDC, SMTP, 발표자료 연동의 비밀값을 **다시 입력**합니다.
3. 사용자 개인 키는 `사용자 관리 → 키 관리`에서 회전시킵니다.

문서 본문은 봉인 대상이 아니므로 그대로 남습니다.

> 키는 데이터베이스 백업과 **다른 곳**에 보관하세요. 같은 곳에 두면 하나를 잃을 때 둘 다 잃습니다.

## 백업

### 정기 백업

```bash
docker compose exec -T postgres \
  pg_dump -U muni -d muni --format=custom --compress=9 \
  > muni-$(date +%Y%m%d).dump
```

`--format=custom`은 선택적 복원을 가능하게 하고 압축이 들어갑니다.

키는 따로 보관합니다. 예를 들어:

```bash
grep '^ENCRYPTION_KEY=' .env > muni-key-$(date +%Y%m%d).txt
chmod 600 muni-key-*.txt
```

### 백업이 실제로 복원되는지 확인

**복원해 본 적 없는 백업은 백업이 아닙니다.** 분기에 한 번은 다른 데이터베이스에 실제로 복원해 보세요.

```bash
createdb -U postgres muni_restore_test
pg_restore -U postgres -d muni_restore_test --no-owner muni-20260101.dump
psql -U postgres -d muni_restore_test -c "SELECT count(*) FROM documents;"
dropdb -U postgres muni_restore_test
```

### 백업 크기 줄이기

첨부가 데이터베이스에 들어 있어 백업이 커집니다. `서비스 관리 → 보존 정책`에서 휴지통과 오래된 버전을 정리하면 크기가 줄어듭니다. 정리는 되돌릴 수 없으니, **켜기 전에 미리보기로 규모를 확인**하세요.

## 복구

```bash
docker compose down
docker volume rm muni_postgres_data      # 볼륨 이름은 설치에 따라 다릅니다
docker compose up -d postgres
docker compose exec -T postgres pg_restore -U muni -d muni --clean --if-exists < muni-20260101.dump
docker compose up -d
```

복원한 데이터베이스와 **같은 시점의 `ENCRYPTION_KEY`**를 사용해야 합니다. 키가 다르면 서비스는 기동하지만 비밀값을 열지 못합니다.

기동할 때 마이그레이션이 자동으로 적용되므로, 백업보다 새로운 버전의 이미지로 복원해도 됩니다. 반대 방향(구버전 이미지로 신버전 데이터 복원)은 지원하지 않습니다.

## 마스터 키 교체

정기 교체나 유출 대응으로 키를 바꾸려면, 현재 키로 열 수 있는 동안 값을 **다시 입력**하는 방식으로 합니다.

1. 현재 설정값을 확인해 둡니다(AI, OIDC, SMTP, 발표자료 연동).
2. 새 키로 `ENCRYPTION_KEY`를 바꾸고 재기동합니다.
3. 비밀값을 다시 입력합니다.
4. `사용자 관리 → 키 관리`에서 개인 키를 회전시킵니다.

봉인된 값을 옛 키로 열어 새 키로 다시 봉인하는 자동 절차는 아직 없습니다. 필요하면 위 순서가 그 일을 대신합니다.

## 사고 시 확인 순서

| 증상 | 먼저 볼 곳 |
| --- | --- |
| 기동하지 않음 | `docker compose logs muni` — 마이그레이션 실패인지 DB 연결 실패인지 |
| 로그인은 되는데 AI·메일이 안 됨 | `서비스 관리 → 운영 현황`의 연결 상태 |
| 특정 문서만 열리지 않음 | `서비스 관리 → 문서 관리`에서 소유자와 휴지통 여부 |
| 디스크가 찬다 | 보존 정책, 그리고 `SELECT pg_size_pretty(pg_total_relation_size('attachments'));` |
| 누가 무엇을 했는지 | `서비스 관리 → 감사 로그`, CSV로 내려받기 |

## 알아두면 좋은 것

- **PDF 내보내기**는 컨테이너 안의 Chromium을 씁니다. 동시 실행 수는 `MUNI_PDF_CONCURRENCY`로 제한합니다(기본 2).
- **협업 상태**(`collab_updates`)는 편집 중에만 쌓이고, 문서를 여는 클라이언트가 주기적으로 하나로 접습니다.
- **감사 로그와 AI 호출 기록**은 보존 정책을 설정하기 전까지 무한히 쌓입니다.
- **메일**은 관리자가 지정한 SMTP 서버로만 나갑니다. 지정하지 않으면 아무것도 발송되지 않습니다.
