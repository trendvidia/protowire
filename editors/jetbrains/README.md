# PXF — Proto eXpressive Format (JetBrains IDEs)

Syntax highlighting **and inline parse-error squiggles** for `.pxf` files
in any JetBrains IDE — IntelliJ IDEA, GoLand, PyCharm, RustRover,
WebStorm, CLion, RubyMine, PhpStorm, Rider, Android Studio.

Two install paths are supported. **Most users want option A.**

## What you get

- TextMate-based syntax highlighting (token colors).
- **Live syntax validation**: the bundled `protowire-java` parser runs on
  every edit and flags malformed PXF (unclosed strings, unbalanced
  braces, bad numeric literals, …) with a red squiggle at the exact line
  and column reported by the parser.
- A **New → PXF File** entry in the project context menu, with a
  `@type ${TYPE_NAME}` starter template.
- Bracket matching, auto-closing pairs, brace folding, and the standard
  ⌘/ comment toggle for `#`, `//`, and `/* */`.

> **Not yet — coming in a follow-up:** schema-aware validation
> (field-not-found, type mismatch, missing-required). That requires a
> per-project descriptor-set setting plus the rest of the protowire-java
> dependency footprint; tracked alongside the packaging refactor below.

## Option A — Install the prebuilt plugin (recommended)

A ready-to-install plugin `.zip` ships at
[`plugin/dist/pxf-jetbrains-0.1.1.zip`](plugin/dist/pxf-jetbrains-0.1.1.zip).

1. In your IDE, open **Settings / Preferences → Plugins**.
2. Click the **⚙** (gear) icon → **Install Plugin from Disk…**.
3. Pick `editors/jetbrains/plugin/dist/pxf-jetbrains-0.1.1.zip`.
4. Restart the IDE when prompted.

The plugin bundles the TextMate grammar and registers it on first project
open — no need to use "Add Bundle" by hand. Open any `.pxf` file and
highlighting should appear.

## Option B — Add the raw TextMate bundle

If you'd rather not install a plugin (or you're using TextMate / Sublime
Text 3+), the bundle directory itself is also valid:

1. **Settings / Preferences → Editor → TextMate Bundles**.
2. Click **+** (Add Bundle) and select
   [`pxf.tmbundle`](pxf.tmbundle) (the directory itself, not a file).
3. Click **Apply / OK**.

## Why two artifacts?

JetBrains' "Install Plugin from Disk" is the standard install flow and
survives IDE updates without re-pointing at a filesystem path. The raw
TextMate bundle exists because (a) it's the canonical source the plugin
itself ships, (b) it works in editors beyond JetBrains, and (c) it's
useful for grammar development.

## Layout

```
editors/jetbrains/
├── pxf.tmbundle/                    # raw TextMate bundle (Option B)
│   ├── info.plist
│   ├── Syntaxes/
│   │   ├── PXF.tmLanguage           # plist (what JetBrains' TextMate plugin reads)
│   │   └── pxf.tmLanguage.json      # JSON (newer JetBrains, TextMate, Sublime)
│   └── Preferences/
│       └── Comments.tmPreferences   # ⌘/ comment toggle
└── plugin/                              # IntelliJ Platform plugin (Option A)
    ├── build.gradle.kts
    ├── settings.gradle.kts
    ├── gradle.properties
    ├── gradlew, gradlew.bat, gradle/wrapper/
    ├── libs/
    │   └── protowire-pxf.jar            # vendored from protowire-java :pxf
    │                                    # (see "Refreshing the parser jar" below)
    ├── src/main/
    │   ├── kotlin/com/trendvidia/pxf/
    │   │   ├── PxfBundleActivity.kt     # registers the TextMate bundle
    │   │   ├── PxfAnnotator.kt          # parse-error squiggles
    │   │   └── actions/NewPxfFileAction.kt
    │   └── resources/
    │       ├── META-INF/plugin.xml
    │       ├── fileTemplates/internal/PXF File.pxf.ft
    │       └── pxf.tmbundle/            # bundled grammar copy
    └── dist/
        └── pxf-jetbrains-0.1.1.zip      # prebuilt plugin
```

## Rebuilding the plugin

Requires JDK 21 (the build downloads its own Gradle and the IntelliJ
Platform SDK on first run, ~600MB):

```bash
cd editors/jetbrains/plugin
./gradlew buildPlugin
cp build/distributions/pxf-jetbrains-0.1.1.zip dist/
```

## Refreshing the parser jar (temporary workflow)

The plugin depends on the PXF parser shipped by **protowire-java**'s
`:pxf` module. Until protowire-java is published to Maven Central, the
parser jar is **vendored** at
[`plugin/libs/protowire-pxf.jar`](plugin/libs/protowire-pxf.jar) and
refreshed by a script:

```bash
bash scripts/refresh_jetbrains_parser_jar.sh
cd editors/jetbrains/plugin && ./gradlew buildPlugin
```

The script expects the sibling `protowire-java/` checkout next to this
repo, runs `./gradlew :pxf:jar` over there, and copies the resulting jar
into `plugin/libs/`.

> **TODO (packaging refactor)**: switch to a normal Maven coordinate
> `implementation("com.trendvidia.protowire:pxf:<version>")` once
> protowire-java is published. At that point `plugin/libs/`, the
> `files(...)` dependency in `build.gradle.kts`, and this entire section
> all go away. Marked in `build.gradle.kts` with a `TODO(packaging):`.

## Keeping the grammar in sync

The canonical grammar lives at
[`editors/vscode/syntaxes/pxf.tmLanguage.json`](../vscode/syntaxes/pxf.tmLanguage.json).
After editing it:

```bash
python3 scripts/sync_jetbrains_grammar.py   # writes both bundle copies
cd editors/jetbrains/plugin && ./gradlew buildPlugin
```

The sync script is idempotent — re-running with no source change produces
byte-identical output, and it now writes both the standalone `.tmbundle`
and the copy embedded in the plugin's resources.
