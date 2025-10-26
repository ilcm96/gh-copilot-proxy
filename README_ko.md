# [gh-copilot-proxy](https://github.com/ilcm96/gh-copilot-proxy)

[English](README.md) | 한국어

## 개요

`gh-copilot-proxy` 는 GitHub Copilot API 를 개인 액세스 토큰 기반으로 사용할 수 있도록 하는 프록시 서버입니다.

- **토큰 자동 로드** : 환경 변수(`COPILOT_OAUTH_TOKEN`)를 우선 사용하며, 없으면 `$HOME/.config/github-copilot` (또는 Windows 의
  `AppData/Local`) 내의 기존 설정 파일에서 `"oauth_token"`을 검색합니다.
- **토큰 수명 관리** : `internal/auth` 가 Copilot 토큰을 메모리에서 유지하면서 자동 갱신합니다.
- **스트리밍 처리** : SSE 기반 응답과 비 스트리밍 응답을 모두 지원합니다.
- **OpenAI/Anthropic 호환** : OpenAI 및 Anthropic 스타일의 클라이언트 요청을 모두 처리합니다.

## 디렉터리 구조

```
.
├── cmd/server/main.go          # 서버 초기화 및 실행
└── internal
    ├── auth                    # Copilot 토큰 관리 및 자동 갱신
    ├── proxy                   # 라우팅, 인증 미들웨어, 업스트림 포워딩
    ├── adapter                 # Anthropic/OpenAI 변환 및 SSE 처리
    └── httpx                   # HTTP 유틸리티 (CORS, 헤더 복사 등)
```

## 빌드 및 실행

### 로컬

```bash
go build -o gh-copilot-proxy cmd/server/main.go
API_KEY=YOUR_API_KEY PORT=4000 ./gh-copilot-proxy
```

### 컨테이너

```bash
API_KEY=$(jq -r 'first(.[] | select(has("oauth_token")) | .oauth_token)' \
    ~/.config/github-copilot/apps.json)

docker build -t gh-copilot-proxy .

docker run -d --name gh-copilot-proxy \
    -e COPILOT_OAUTH_TOKEN="${API_KEY}" \
    -e API_KEY=YOUR_API_KEY \
    -e PORT=4000 \
    -p 4000:4000 \
    gh-copilot-proxy
```

### 환경 변수

| 이름                  | 기본값        | 설명                                                                                                                |
| --------------------- | ------------- | ------------------------------------------------------------------------------------------------------------------- |
| `COPILOT_OAUTH_TOKEN` | 없음 (필수\*) | GitHub Copilot OAuth 토큰. 비어 있으면 기존 GitHub CLI/VS Code 환경(`apps.json`, `hosts.json`)에서 자동 검색합니다. |
| `API_KEY`             | 자동 생성     | 프록시 접근 제어용 Bearer 토큰. 비어 있으면 기동 시 암호화 난수로 생성됩니다.                                       |
| `PORT`                | `4000`        | 바인딩할 포트. 예: `5000`                                                                                           |

- 컨테이너 환경에서는 파일 시스템 권한 이슈로 `COPILOT_OAUTH_TOKEN` 사용을 권장합니다.
- GitHub Copilot OAuth 토큰을 얻기 위해서는 다음 명령어를 실행하세요:
    ```bash
    API_KEY=$(jq -r 'first(.[] | select(has("oauth_token")) | .oauth_token)' \
        ~/.config/github-copilot/apps.json)
    ```

## 지원 엔드포인트

- **OpenAI**
  - `/v1/chat/completions`
  - `/chat/completions`
  - `/v1/embeddings`
  - `/embeddings`
- **Anthropic**
  - `/v1/messages`
  - `/messages`

모든 엔드포인트는 `Authorization: Bearer <API_KEY>` 헤더가 필요합니다.
