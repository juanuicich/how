# how

A tiny TUI that turns natural language into a shell command, powered by Claude Code.

```
$ how find processes using port 3000
```

Pick a key: run it, refine it, or get an explanation in Markdown. That's the whole thing.

## Requirements

- macOS or Linux
- The [`claude`](https://docs.claude.com/claude-code) CLI on your `PATH`, signed in

`how` shells out to `claude -p --model sonnet`, so if Claude Code works, `how` works.

## Install

### Homebrew (macOS, Linux)

```sh
brew install juanuicich/tap/how
```

### From a release binary

Grab the right archive for your OS/arch from the [Releases page](https://github.com/juanuicich/how/releases), then:

```sh
tar -xzf how_*_darwin_arm64.tar.gz
sudo mv how /usr/local/bin/
```

On macOS, binaries downloaded from the web are quarantined by Gatekeeper. If you see *"how cannot be opened because the developer cannot be verified"*, clear the quarantine attribute:

```sh
xattr -d com.apple.quarantine /usr/local/bin/how
```

### From source

```sh
go install github.com/juanuicich/how@latest
```

Or clone and build:

```sh
git clone https://github.com/juanuicich/how
cd how
go build -o how .
```

## Usage

```sh
how <what you want to do>
```

Once Claude responds with a command:

| key      | action                                  |
| -------- | --------------------------------------- |
| `enter`  | run the command                         |
| `tab`    | refine — add more context, iterate      |
| `e`      | explain the command in Markdown         |
| `q`/`esc`| quit                                    |

Example session:

```
$ how list files modified in the last hour
find . -type f -mmin -60
[enter] run  [tab] refine  [e] explain  [q] quit
```

Hit `tab` to refine ("only inside src/"), `e` to learn what the flags mean, `enter` to run it. Output streams to your terminal as normal — `how` exits with the command's exit code.

## Flags

```
-h, --help       show help
-v, --version    print version
```

Everything else is treated as your question.

## Releasing

This repo publishes via [GoReleaser](https://goreleaser.com/) on every `v*` tag. To cut a release:

```sh
git tag v0.1.0
git push origin v0.1.0
```

GitHub Actions takes it from there: cross-compiled binaries land on the Releases page and the Homebrew formula at [`juanuicich/homebrew-tap`](https://github.com/juanuicich/homebrew-tap) is updated automatically.

The workflow needs one secret on this repo: `HOMEBREW_TAP_GITHUB_TOKEN`, a PAT with `contents: write` on the tap repo. Without it the binaries still ship; only the brew bump is skipped.

## License

MIT. See [LICENSE](LICENSE).
