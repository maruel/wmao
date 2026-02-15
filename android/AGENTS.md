# Android Guidelines

Kotlin/Compose Android app for caic. Voice-first companion for managing coding agents.

## Current State

Scaffolded project with a single `MainActivity`. No SDK, no Compose, no Hilt yet.
The app currently uses View-based layout (`activity_main.xml`), not Compose.

## Target Architecture

See `docs/` for full design specs:
- `docs/sdk-design.md` — Kotlin SDK: generated API client + types
- `docs/app-design.md` — App: screens, voice mode, state management

### Layer Summary

```
UI (Compose screens) → ViewModels (StateFlow) → Repositories → SDK (ApiClient)
                                                             → Gemini Live (voice)
                                                             → DataStore (settings)
```

No business logic in Compose. No Android dependencies in the SDK module.

## Conventions

- **Package**: `com.fghbuild.caic` (app), SDK module TBD
- **DI**: Hilt (when added)
- **Serialization**: `kotlinx.serialization` (not Gson/Moshi)
- **Networking**: OkHttp + OkHttp SSE
- **Async**: Coroutines + `StateFlow` (not LiveData, not RxJava)
- **Navigation**: Compose Navigation with type-safe routes
- **Compose naming**: `PascalCase` for composables (detekt `functionPattern` allows this)
- **Line length**: 120 chars (detekt config)
- **No wildcard imports** (detekt enforced)

## Build & Lint

```bash
make lint-android   # detekt + Android lint
make android-build  # assembleDebug
make android-test   # JVM unit tests
```

Lint is strict: `warningsAsErrors = true`, `maxIssues: 0`.

## Development Notes

- `minSdk = 34`, `targetSdk = 36`, `compileSdk = 36`
- Version catalog at `gradle/libs.versions.toml`
- detekt config at `detekt.yml`
- The web frontend (SolidJS) in `../frontend/` is the reference implementation for
  screen behavior, event grouping, and formatting. Match it.

## Implementation Order

Follow the design docs in this order:

1. **SDK module** (`docs/sdk-design.md`): types + API client, unit tested on JVM
2. **Screen mode** (`docs/app-design.md` §Screens): Compose UI with real data
3. **Voice mode** (`docs/app-design.md` §Voice): Gemini Live integration

Each step should build, lint, and test clean before proceeding.
