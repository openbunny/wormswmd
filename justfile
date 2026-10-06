set shell := ["bash", "-euo", "pipefail", "-c"]

c_files := "internal/agl/*/*.[ch]"

default:
    @just --list

build:
    go build -ldflags "-X github.com/openbunny/wormswmd/cmd.buildVersion=$(git describe --tags --always --dirty)" -o wormswmd .

snapshot:
    goreleaser release --snapshot --clean --skip=sign

fmt:
    golangci-lint fmt
    clang-format -i {{ c_files }}
    prettier --write '**/*.md' '.github/workflows/*.yml' '**/*.json5' '**/*.jsonc'
    yamlfmt
    taplo fmt

check: check-go check-c check-md check-data

check-go:
    golangci-lint run
    go test -coverprofile=coverage.out ./...
    go-test-coverage --config .testcoverage.yml

check-c:
    clang-format --dry-run -Werror {{ c_files }}
    clang-tidy --quiet {{ c_files }} -- -std=c11 -Iinternal/agl/stub
    cppcheck --quiet --error-exitcode=1 --enable=warning,style,performance,portability --std=c11 -Iinternal/agl/stub internal/agl

check-md:
    markdownlint-cli2
    prettier --check '**/*.md'

check-data:
    actionlint
    zizmor --persona=pedantic .github
    pinact run --check
    prettier --check '.github/workflows/*.yml' '**/*.json5' '**/*.jsonc'
    yamlfmt -lint
    yamllint --strict .
    taplo fmt --check
    taplo lint
    reuse lint
