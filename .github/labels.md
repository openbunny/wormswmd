# Label taxonomy

Three prefixes: `type/*` classifies an issue, `area/*` locates it, `status/*`
tracks it through triage. Every issue and PR carries exactly one `type/*`
label; `area/*` and `status/*` are added during triage.

## type/*

| Label           | Meaning                                               |
| --------------- | ----------------------------------------------------- |
| `type/bug`      | A command behaves differently from what it documents. |
| `type/feature`  | A new command, flag, or behaviour.                    |
| `type/docs`     | README, CONTRIBUTING, or other documentation only.    |
| `type/chore`    | Build, dependency, or repository maintenance.         |
| `type/security` | A vulnerability report or hardening change.           |

## area/*

| Label               | Covers                                                                                      |
| ------------------- | ------------------------------------------------------------------------------------------- |
| `area/agl`          | The AGL stub in `internal/agl/stub` and its build                                           |
| `area/apply`        | Bundle mutation in `internal/apply`, `internal/bundle`, and `internal/tree`                 |
| `area/bundle`       | Mach-O install names and `Info.plist` rewriting in `internal/macho` and `internal/plistfix` |
| `area/check`        | The app and save checks in `internal/check` and `internal/game`                             |
| `area/qt`           | Qt framework download, checksum pin, and install-name rewrite                               |
| `area/saves`        | Save backup and restore in `internal/saves` and `internal/backup`                           |
| `area/support`      | The support report in `internal/support`                                                    |
| `area/dependencies` | The Qt archive pin in `internal/qt`, `go.mod`, and tool pins                                |
| `area/ci`           | GitHub Actions workflows and the `justfile` gates                                           |
| `area/release`      | Release configuration and artefact signing                                                  |
| `area/docs`         | README, CONTRIBUTING, and other tracked docs                                                |

## status/*

| Label                 | Meaning                                                         |
| --------------------- | --------------------------------------------------------------- |
| `status/needs-triage` | Default label on a new issue; a maintainer has not reviewed it. |
| `status/confirmed`    | A maintainer reproduced the bug or accepted the proposal.       |
| `status/blocked`      | Waiting on an external dependency or decision.                  |
| `status/wontfix`      | Closed without a change; the issue states why.                  |
