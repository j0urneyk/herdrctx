# herdrctx

[English](README.md)

`herdrctx`는 로컬 [Herdr](https://herdr.dev/) 세션을 관리하는 터미널 UI입니다. 세션을 검색·필터링하고 즐겨찾기를 상단에 모으거나 전체 세션 정보를 확인할 수 있습니다. 하나의 목록에서 키보드로 세션에 접속하거나 새로 만들고 중지·삭제합니다. 목록은 3초마다 자동으로 갱신되며, Herdr에서 분리(detach)하면 다시 표시됩니다.

## 설치

[Herdr](https://herdr.dev/) 0.6.5 이상을 먼저 설치하고 `PATH`에 등록해 터미널에서 `herdr`를 실행할 수 있도록 하세요. `herdrctx`는 macOS와 Linux의 x86_64, arm64 환경을 지원하며, 대화형 터미널이 필요합니다.

Homebrew로 설치하려면:

```sh
brew install j0urneyk/tap/herdrctx
```

Go 1.26.3으로 소스에서 설치할 수도 있습니다.

```sh
go install github.com/j0urneyk/herdrctx/cmd/herdrctx@latest
```

Go의 바이너리 디렉터리(`go env GOBIN`, 미설정 시 `$(go env GOPATH)/bin`)가 `PATH`에 포함되어 있어야 합니다.

### Herdr 플러그인

Herdr 0.7.0 이상에서는 [플러그인 관리자](https://herdr.dev/docs/cli-reference/#plugins)로 릴리스 바이너리를 설치할 수 있습니다. `git`, `curl`, `tar`, `sha256sum` 또는 `shasum`이 필요하며 Go는 필요하지 않습니다.

```sh
herdr plugin install j0urneyk/herdrctx
export PATH="$HOME/.local/bin:$PATH"
```

플러그인은 독립 실행형 CLI를 설치하며, Herdr 내부 pane이나 action을 추가하지 않습니다. 설치할 때 Herdr 서버를 실행할 필요는 없습니다. 설치기는 압축 파일의 SHA-256을 릴리스 체크섬과 대조합니다. 새 터미널에서도 실행하려면 PATH 설정을 셸 설정 파일에 추가하세요. 설치기는 셸 설정을 수정하지 않습니다. 설치 경로 변경, 버전 선택, 제거 방법은 [플러그인 설치 관리](#플러그인-설치-관리)를 참고하세요.

## 사용법

**Herdr 밖의 터미널에서** 실행하세요.

```sh
herdrctx
```

`↑` / `↓`로 실행 중인 세션을 선택하고 `enter`를 누르면 접속합니다. 새로 만들려면 `n`을 누르고 `work` 같은 이름을 입력한 뒤 `enter`를 누르세요. 현재 디렉터리에 세션을 만들고 바로 접속합니다. Herdr에서 분리(detach)하면 다시 목록으로 돌아옵니다.

### 단축키

| 키 | 동작 |
| --- | --- |
| `↑` / `k`, `↓` / `j` | 위·아래로 이동 |
| `enter` / `a` | 선택한 세션에 접속 (기본값: 실행 중인 세션만) |
| `/` | 세션 검색 |
| `f` | 전체·실행 중·중지됨 상태 필터 전환 |
| `o` | 이름순·실행 중 우선 정렬 전환 |
| `p` | 선택한 세션의 즐겨찾기 등록·해제 |
| `i` | 세션 전체 정보 보기 |
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

새 세션 이름은 1~64자로, ASCII 영문 또는 숫자로 시작해야 합니다. 이후에는 ASCII 영문·숫자와 `-`, `_`, `.`만 사용할 수 있습니다. `help`는 예약어입니다. 기존 세션은 이름이 생성 규칙에 맞지 않아도 목록에 표시됩니다.

### 필터, 즐겨찾기와 상세 보기

처음에는 전체 세션을 이름순으로 표시합니다. `f`로 전체·실행 중·중지됨 세션을 순환하고, `o`로 이름순·실행 중 우선 정렬을 전환합니다. 현재 조건과 일치하는 세션 수는 목록 위에 표시됩니다. 상태 필터와 검색을 함께 적용할 수 있으며, 검색 중 `esc`는 검색어만 지웁니다. 필터와 정렬은 갱신 후에도 유지되고 앱 재실행 시 초기화됩니다.

`p`로 선택한 세션을 즐겨찾기에 등록하면 `⭐`가 표시되고 조건에 맞는 다른 세션보다 위에 모입니다. 각 그룹 안에서는 선택한 정렬이 적용되며, 즐겨찾기도 현재 검색과 상태 필터에 맞아야 표시됩니다. 즐겨찾기는 정확한 세션 이름으로 저장되고 세션을 삭제해도 남습니다. 같은 이름으로 다시 만들면 즐겨찾기를 이어받습니다. 기본 세션의 기존 `*` 표시는 별도로 유지됩니다.

`i`로 세션 전체 정보를 엽니다. `↑` / `↓`, `PgUp` / `PgDn`으로 스크롤하고 `enter`, `esc`, `q`로 닫습니다. 상세 정보는 갱신을 따라가며 세션이 사라지거나 갱신에 실패하면 이를 표시합니다. **Directory** 열과 디렉터리 검색은 프로젝트 작업 디렉터리가 아닌 Herdr의 세션 상태 저장 경로를 사용합니다.

### 설정 저장

즐겨찾기는 운영체제의 사용자 설정 디렉터리 아래 `herdrctx/preferences.json`에 저장됩니다.

| 운영체제 | 기본 경로 |
| --- | --- |
| macOS | `~/Library/Application Support/herdrctx/preferences.json` |
| Linux | `$XDG_CONFIG_HOME/herdrctx/preferences.json`, 미설정 시 `~/.config/herdrctx/preferences.json` |

파일은 첫 저장 시 만들어집니다. 저장 실패 시 기존 설정을 유지하고 오류 창을 표시합니다. 원자적 파일 교체와 앱 간 잠금을 사용하므로 다른 인스턴스가 저장 중이면 다시 시도하세요. 저장할 때 파일을 다시 읽고 선택한 즐겨찾기만 변경하므로 다른 인스턴스의 변경도 보존합니다. 변경을 계속 감시하지는 않습니다.

시작 시 설정을 읽지 못하면 원본 파일을 보존하고 경고한 뒤 저장 기능을 비활성화합니다. 일반 세션 관리는 계속 사용할 수 있습니다. 파일을 수정하고 재실행하면 저장 기능을 다시 사용할 수 있습니다. 사라진 세션의 즐겨찾기를 정리하거나 JSON을 직접 편집할 때는 먼저 herdrctx를 종료하세요. 버전이 있는 저장 형식과 구현 세부 사항은 [UI 동작 문서](docs/ui.md#saved-preferences)에 설명되어 있습니다.

### 세션 중지와 삭제

**세션을 중지하면 안에서 실행 중인 셸이나 서버 등의 프로세스도 종료될 수 있습니다.** 중지와 삭제는 실행 전에 확인을 받습니다. `y` / `enter`로 실행하고, `n` / `esc`로 취소합니다.

삭제하면 저장된 세션 상태가 지워집니다. 실행 중인 세션은 먼저 중지해야 삭제할 수 있고, 기본 세션은 삭제할 수 없습니다.

### 중지된 세션에 접속

기본적으로 `enter` / `a`는 실행 중인 세션에만 접속합니다. 중지된 세션을 선택하면 Herdr를 실행하지 않고 안내창을 표시합니다. 중지된 세션의 시작과 접속을 허용하려면 `herdrctx --allow-stopped-attach`로 실행하거나 `HERDRCTX_ALLOW_STOPPED_ATTACH=1`을 설정하세요. `--allow-stopped-attach=false`는 환경변수보다 우선하여 차단합니다. 허용 시 중지된 세션의 하단 설명은 `start and attach`로 바뀌며, 추가 확인 없이 시작하고 접속합니다.

이 설정은 세션 목록의 접속 동작에 적용됩니다. `n`, `N`은 기존처럼 세션을 생성하거나 재사용하므로 같은 이름의 중지된 세션을 다시 시작할 수 있습니다. 차단은 최근 목록 상태를 기준으로 하므로, 실행 중으로 표시된 세션이 갱신과 접속 사이에 중지되면 Herdr가 다시 시작할 수 있습니다.

## 설정

갱신 주기를 바꾸거나 Herdr 실행 파일을 직접 지정할 수 있습니다.

```sh
herdrctx --interval 5s
herdrctx --herdr-bin /opt/homebrew/bin/herdr
```

갱신 주기는 최소 `500ms`입니다. Herdr 경로는 `HERDRCTX_HERDR_BIN` 환경 변수로도 설정할 수 있으며, 명령줄 옵션이 우선합니다. 디렉터리 자동완성을 비롯한 전체 옵션은 `herdrctx --help`에서 확인하세요.

Herdr 안에서 실행하면 세션 접속과 생성이 기본적으로 차단됩니다. 중첩 실행을 허용하려면 Herdr의 [`experimental.allow_nested`](https://herdr.dev/docs/config-reference/#experimental) 설정을 켠 뒤, `herdrctx --allow-nested`로 실행하거나 `HERDRCTX_ALLOW_NESTED=1`을 설정하세요.

## 플러그인 설치 관리

설치기는 선택한 체크아웃의 [`herdr-plugin.toml`](herdr-plugin.toml)에 적힌 버전을 `~/.local/bin/herdrctx`에 설치합니다. 같은 설치 명령을 다시 실행하면 해당 버전으로 바이너리를 교체합니다. 다른 디렉터리에 설치하려면:

```sh
HERDRCTX_INSTALL_DIR=/your/bin herdr plugin install j0urneyk/herdrctx
```

지정한 디렉터리를 `PATH`에 추가하세요. Homebrew나 Go로도 설치했다면 `command -v herdrctx`로 셸이 실행하는 바이너리를 확인하세요.

특정 리비전은 `herdr plugin install j0urneyk/herdrctx --ref <tag-or-commit>`으로 선택합니다. 해당 리비전에는 매니페스트와 설치기가 있어야 하고, 매니페스트에 적힌 바이너리 릴리스도 공개되어 있어야 합니다. 설치 가능한 버전과 이전 리비전은 [플러그인 버전과 배포](docs/releases.md#plugin-versions-and-publication)를 참고하세요.

`herdr plugin uninstall herdrctx`는 관리되는 체크아웃과 등록 정보를 제거하지만 바이너리는 남깁니다. 기본 경로라면 `rm "$HOME/.local/bin/herdrctx"`로 제거하세요. `HERDRCTX_INSTALL_DIR`을 지정했다면 해당 디렉터리의 `herdrctx`를 제거하세요.

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
make test-integration
```

통합 테스트도 Go로 작성되어 있으며 `integration` 빌드 태그로 분리됩니다. `make test-integration`은 앱을 빌드하고 로컬 통합 테스트를 실행하며, 최초 실행 시 체크섬이 고정된 Herdr 테스트 바이너리를 내려받습니다. 특정 테스트만 실행하려면 `make test-integration INTEGRATION_ARGS='-run ^TestNavigation$'`를 사용하세요.

CI에서는 Ubuntu 24.04와 macOS 15의 두 지원 아키텍처를 검증합니다. 실제 Herdr 세션 테스트를 포함해 변경에 맞는 검사를 고르려면 [테스트 문서](docs/testing.md)를, 배포와 릴리스 빌드는 [릴리스 문서](docs/releases.md)를 참고하세요.

사용자에게 영향을 주는 변경 사항은 [변경 이력](CHANGELOG.md)에 기록합니다.
