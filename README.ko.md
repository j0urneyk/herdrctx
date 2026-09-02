# herdrctx

[English](README.md)

`herdrctx`는 로컬 [Herdr](https://herdr.dev/) 세션을 한 화면에서 관리하는 터미널 UI입니다. 키보드로 세션을 골라 접속하고, 새로 만들거나 중지·삭제할 수 있습니다. 목록은 3초마다 자동으로 갱신됩니다.

## 설치

Herdr 0.6.5 이상이 필요하며, `PATH`에 등록되어 터미널에서 `herdr`를 실행할 수 있어야 합니다. `herdrctx`는 macOS와 Linux의 x86_64, arm64 환경을 지원합니다.

CI에서는 Ubuntu 24.04와 macOS 15의 두 아키텍처를 검증합니다. 검사 항목과 검증 범위는 [테스트 문서](docs/testing.md)를 참고하세요.

Homebrew로 설치하려면:

```sh
brew install j0urneyk/tap/herdrctx
```

Go 1.26.3으로 소스에서 설치할 수도 있습니다.

```sh
go install github.com/j0urneyk/herdrctx/cmd/herdrctx@latest
```

### Herdr 플러그인

Herdr 0.7.0 이상에서는 [플러그인 관리자](https://herdr.dev/docs/plugins/)로 릴리스 바이너리를 설치할 수 있습니다. `git`, `curl`, `tar`, `sha256sum` 또는 `shasum`이 필요하며 Go는 필요하지 않습니다.

```sh
herdr plugin install j0urneyk/herdrctx
export PATH="$HOME/.local/bin:$PATH"
herdrctx
```

설치 후 **Herdr 밖의 터미널에서 `herdrctx`를 실행**하세요. 플러그인은 검색과 설치를 제공하며, 세션 내부 pane이나 action을 추가하지 않습니다. 설치할 때 Herdr 서버를 실행할 필요는 없습니다. CLI 자체는 계속 Herdr 0.6.5 이상을 지원합니다.

설치기는 선택한 checkout의 `herdr-plugin.toml`에 적힌 버전을 SHA-256으로 검증해 `~/.local/bin/herdrctx`에 설치합니다. 다른 위치는 `HERDRCTX_INSTALL_DIR=/your/bin herdr plugin install j0urneyk/herdrctx`로 지정하고, 해당 디렉터리를 shell의 PATH에 추가하세요. Homebrew나 Go로도 설치했다면 `command -v herdrctx`로 실행 경로를 확인하세요. 설치기는 shell 설정을 수정하지 않습니다.

같은 설치 명령을 다시 실행하면 선택한 manifest 버전으로 바이너리를 교체합니다. 특정 revision은 `herdr plugin install j0urneyk/herdrctx --ref <tag-or-commit>`으로 지정합니다. 해당 revision에는 manifest와 설치기가 있어야 하고, 바이너리 릴리스도 공개되어 있어야 합니다. 기존 `v0.0.2` 태그에는 플러그인 파일이 없어 `--ref`로 사용할 수 없습니다. 첫 manifest는 해당 버전의 바이너리만 재사용합니다.

`herdr plugin uninstall herdrctx`는 관리되는 checkout과 등록 정보를 제거합니다. 설치한 바이너리는 남으므로, 기본 경로라면 `rm "$HOME/.local/bin/herdrctx"`로 직접 제거하세요. `HERDRCTX_INSTALL_DIR`을 지정했다면 해당 디렉터리의 `herdrctx`를 제거하세요.

## 사용법

```sh
herdrctx
```

세션을 선택하고 `enter`를 누르면 접속합니다. Herdr에서 분리(detach)하면 다시 세션 목록으로 돌아옵니다. 새 세션을 만들 때도 바로 접속합니다.

### 단축키

| 키 | 동작 |
| --- | --- |
| `↑` / `k`, `↓` / `j` | 위·아래로 이동 |
| `enter` / `a` | 선택한 세션에 접속 |
| `/` | 세션 검색 |
| `n` | 현재 디렉터리에서 새 세션을 만들고 접속 |
| `N` | 디렉터리를 지정해 새 세션을 만들고 접속 |
| `s` | 확인 후 선택한 세션 중지 |
| `d` | 확인 후 선택한 세션 삭제 |
| `r` | 목록 새로고침 |
| `?` | 도움말 열기·닫기 |
| `q` / `ctrl+c` | 종료 |

경고나 오류 창은 `enter`, `esc`, `q`로 닫습니다. 창이 열려 있을 때도 `ctrl+c`는 앱을 종료합니다.

### 세션 검색과 생성

`/`를 누르고 이름 일부를 입력하면 일치하는 세션만 표시됩니다. 검색 중 `tab`으로 이름과 디렉터리 검색을 전환할 수 있습니다. `enter`는 검색 조건을 유지한 채 입력창을 닫고, `esc`는 검색을 해제합니다.

`N`으로 만들 때는 디렉터리를 반드시 입력해야 합니다. 없는 디렉터리는 자동으로 생성됩니다. `↑` / `↓`로 입력 항목을 이동하거나 자동완성 후보를 고르고, `tab`으로 후보를 적용합니다. `esc`는 자동완성 목록부터 닫고, 한 번 더 누르면 생성을 취소합니다.

새 세션 이름은 1~64자로, 영문 또는 숫자로 시작해야 합니다. 이후에는 영문·숫자와 `-`, `_`, `.`만 사용할 수 있습니다. 예를 들어 `my-project`, `work_1`, `v1.2`는 허용합니다. `help`는 예약어입니다. 기존 세션은 이름이 생성 규칙에 맞지 않아도 목록에 표시됩니다.

### 세션 중지와 삭제

**세션을 중지하면 안에서 실행 중인 셸이나 서버 등의 프로세스도 종료될 수 있습니다.** 중지와 삭제는 실행 전에 확인을 받습니다.

삭제하면 저장된 세션 상태가 지워집니다. 실행 중인 세션은 먼저 중지해야 삭제할 수 있고, 기본 세션은 삭제할 수 없습니다.

## 설정

갱신 주기를 바꾸거나 Herdr 실행 파일을 직접 지정할 수 있습니다.

```sh
herdrctx --interval 5s
herdrctx --herdr-bin /opt/homebrew/bin/herdr
```

갱신 주기는 최소 `500ms`입니다. Herdr 경로는 `HERDRCTX_HERDR_BIN` 환경 변수로도 설정할 수 있으며, 명령줄 옵션이 우선합니다. 디렉터리 자동완성을 비롯한 전체 옵션은 `herdrctx --help`에서 확인하세요.

Herdr 안에서 실행하면 세션 접속과 생성이 기본적으로 차단됩니다. 중첩 실행이 필요하다면 Herdr의 `experimental.allow_nested` 설정을 켜고, `herdrctx --allow-nested`로 실행하거나 `HERDRCTX_ALLOW_NESTED=1`을 설정하세요.

## 개발

Go 1.26.3과 `golangci-lint`를 사용합니다. asdf를 쓴다면 저장소에서 `asdf install`로 Go 버전을 맞출 수 있습니다.

```sh
git clone https://github.com/j0urneyk/herdrctx.git
cd herdrctx
go run ./cmd/herdrctx
```

변경 사항 검증과 빌드:

```sh
make test
make vet
make lint
make build
```

CI 검사와 검증 범위는 [테스트 문서](docs/testing.md), 배포와 릴리스 빌드는 [릴리스 문서](docs/releases.md)를 참고하세요.
