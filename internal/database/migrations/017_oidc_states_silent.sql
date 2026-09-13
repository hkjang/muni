-- 조용한 로그인(prompt=none)의 표시.
--
-- prompt=none 으로 시작한 로그인은 제공자에 세션이 없으면 error=login_required 로
-- 돌아온다. 그것은 실패가 아니라 평범한 대답인데, 콜백은 그 요청이 조용한
-- 시도였는지 알 길이 없어 오류로 보여 주었다. 시작할 때 적어 두면 콜백이
-- 거절을 로그인 화면의 평범한 도착으로 넘길 수 있다.
ALTER TABLE oidc_states ADD COLUMN IF NOT EXISTS silent boolean NOT NULL DEFAULT false;
