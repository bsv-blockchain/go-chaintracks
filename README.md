<div align="center">

# ⛓️&nbsp;&nbsp;go-chaintracks

**Real-time Bitcoin SV header orchestration in Go**

<br/>

<a href="https://github.com/bsv-blockchain/go-chaintracks/releases"><img src="https://img.shields.io/github/release-pre/bsv-blockchain/go-chaintracks?include_prereleases&style=flat-square&logo=github&color=black" alt="Release"></a>
<a href="https://golang.org/"><img src="https://img.shields.io/github/go-mod/go-version/bsv-blockchain/go-chaintracks?style=flat-square&logo=go&color=00ADD8" alt="Go Version"></a>
<a href="LICENSE"><img src="https://img.shields.io/badge/license-OpenBSV-blue?style=flat-square&logo=springsecurity&logoColor=white" alt="License"></a>

<br/>

<table align="center" border="0">
  <tr>
    <td align="right">
       <code>CI / CD</code> &nbsp;&nbsp;
    </td>
    <td align="left">
       <a href="https://github.com/bsv-blockchain/go-chaintracks/actions"><img src="https://img.shields.io/github/actions/workflow/status/bsv-blockchain/go-chaintracks/fortress.yml?branch=master&label=build&logo=github&style=flat-square" alt="Build"></a>
       <a href="https://github.com/bsv-blockchain/go-chaintracks/actions"><img src="https://img.shields.io/github/last-commit/bsv-blockchain/go-chaintracks?style=flat-square&logo=git&logoColor=white&label=last%20update" alt="Last Commit"></a>
    </td>
    <td align="right">
       &nbsp;&nbsp;&nbsp;&nbsp; <code>Quality</code> &nbsp;&nbsp;
    </td>
    <td align="left">
       <a href="https://goreportcard.com/report/github.com/bsv-blockchain/go-chaintracks"><img src="https://goreportcard.com/badge/github.com/bsv-blockchain/go-chaintracks?style=flat-square" alt="Go Report"></a>
       <a href="https://codecov.io/gh/bsv-blockchain/go-chaintracks"><img src="https://codecov.io/gh/bsv-blockchain/go-chaintracks/branch/master/graph/badge.svg?style=flat-square" alt="Coverage"></a>
    </td>
  </tr>

  <tr>
    <td align="right">
       <code>Security</code> &nbsp;&nbsp;
    </td>
    <td align="left">
       <a href="https://scorecard.dev/viewer/?uri=github.com/bsv-blockchain/go-chaintracks"><img src="https://api.scorecard.dev/projects/github.com/bsv-blockchain/go-chaintracks/badge?style=flat-square" alt="Scorecard"></a>
       <a href=".github/SECURITY.md"><img src="https://img.shields.io/badge/policy-active-success?style=flat-square&logo=security&logoColor=white" alt="Security"></a>
    </td>
    <td align="right">
       &nbsp;&nbsp;&nbsp;&nbsp; <code>Community</code> &nbsp;&nbsp;
    </td>
    <td align="left">
       <a href="https://github.com/bsv-blockchain/go-chaintracks/graphs/contributors"><img src="https://img.shields.io/github/contributors/bsv-blockchain/go-chaintracks?style=flat-square&color=orange" alt="Contributors"></a>
       <a href="https://github.com/sponsors/bsv-blockchain"><img src="https://img.shields.io/badge/sponsor-BSV-181717.svg?logo=github&style=flat-square" alt="Sponsor"></a>
    </td>
  </tr>
</table>

</div>

<br/>
<br/>

<div align="center">

### <code>Project Navigation</code>

</div>

<table align="center">
  <tr>
    <td align="center" width="33%">
       📦&nbsp;<a href="#-installation"><code>Installation</code></a>
    </td>
    <td align="center" width="33%">
       📚&nbsp;<a href="#-documentation"><code>Documentation</code></a>
    </td>
    <td align="center" width="33%">
       🧪&nbsp;<a href="#-examples--tests"><code>Examples&nbsp;&&nbsp;Tests</code></a>
    </td>
  </tr>
  <tr>
    <td align="center">
       🤝&nbsp;<a href="#-contributing"><code>Contributing</code></a>
    </td>
    <td align="center">
      🛠️&nbsp;<a href="#-code-standards"><code>Code&nbsp;Standards</code></a>
    </td>
    <td align="center">
      ⚡&nbsp;<a href="#-benchmarks"><code>Benchmarks</code></a>
    </td>
  </tr>
  <tr>
    <td align="center">
      🤖&nbsp;<a href="#-ai-usage--assistant-guidelines"><code>AI&nbsp;Usage</code></a>
    </td>
    <td align="center">
       📝&nbsp;<a href="#-license"><code>License</code></a>
    </td>
    <td align="center">
       👥&nbsp;<a href="#-maintainers"><code>Maintainers</code></a>
    </td>
  </tr>
</table>
<br/>

## 📦 Installation

**go-chaintracks** requires a [supported release of Go](https://golang.org/doc/devel/release.html#policy).
```shell script
go get -u github.com/bsv-blockchain/go-chaintracks
```

<br/>

## 📚 Documentation
- **API Reference** – Dive into the godocs at [pkg.go.dev/github.com/bsv-blockchain/go-chaintracks](https://pkg.go.dev/github.com/bsv-blockchain/go-chaintracks)

### Features

- In-memory chain tracking with height and hash indexes
- Chainwork calculation and comparison
- Automatic orphan pruning (keeps last 100 blocks)
- P2P live sync with automatic updates
- Bootstrap sync from CDN or API sources
- REST API with v2 endpoints
- CDN server for hosting header files
- File-based persistence with metadata
- Docker support with docker-compose

### Architecture
- **ChainManager** - Main orchestrator for chain operations
- **BlockHeader** - Extends SDK header with height and chainwork
- **File I/O** - Local storage with seek-based updates
- **P2P Sync** - Live header updates via message bus
- **ChainTracker** - Implements go-sdk interface

<br/>

<details>
<summary><strong><code>Usage as a Library</code></strong></summary>

```go
import "github.com/bsv-blockchain/go-chaintracks/chaintracks"

// Create chain manager with local storage
// Network options: "main", "test", "teratest"
// Optional bootstrap URL for initial sync
cm, err := chaintracks.NewChainManager("main", "~/.chaintracks", "https://node.example.com")
if err != nil {
    log.Fatal(err)
}

// Start P2P sync for automatic updates
ctx := context.Background()
tipChanges, err := cm.Start(ctx)
if err != nil {
    log.Fatal(err)
}

// Listen for tip changes (optional)
go func() {
    for tip := range tipChanges {
        log.Printf("New tip: height=%d hash=%s", tip.Height, tip.Hash())
    }
}()

// Query methods
tip := cm.GetTip()
height := cm.GetHeight()
header, err := cm.GetHeaderByHeight(123456)
header, err := cm.GetHeaderByHash(&hash)

// Cleanup
defer cm.Stop()
```

</details>

<details>
<summary><strong><code>Usage as a Client</code></strong></summary>

```go
import "github.com/bsv-blockchain/go-chaintracks/pkg/chaintracks"

// Connect to remote chaintracks server
client := chaintracks.NewChainClient("http://localhost:3011")

// Start SSE connection for automatic updates
ctx := context.Background()
tipChanges, err := client.Start(ctx)
if err != nil {
    log.Fatal(err)
}

// Listen for tip changes (optional)
go func() {
    for tip := range tipChanges {
        log.Printf("New tip: height=%d hash=%s", tip.Height, tip.Hash)
    }
}()

// Query methods (same interface as ChainManager)
tip := client.GetTip()
height := client.GetHeight()
header, err := client.GetHeaderByHeight(123456)
header, err := client.GetHeaderByHash(&hash)

// Cleanup
defer client.Stop()
```

</details>

<details>
<summary><strong><code>Usage as a Server</code></strong></summary>

```bash
# Build and run
go build -o server ./cmd/server
./server

# Configure via .env file
cp env.docker .env
# Edit .env with your settings

# Or configure via environment variables
CHAINTRACKS_PORT=3011 CHAINTRACKS_CHAINTRACKS_P2P_NETWORK=main ./server
```

Server starts on port 3011 with Swagger UI at `/docs`.

</details>

<details>
<summary><strong><code>Docker Deployment</code></strong></summary>

The easiest way to run go-chaintracks is with Docker:

```bash
# Quick start with default settings
docker compose up -d

# View logs
docker compose logs -f
```

This starts:
- **API Server** on port 3011 - REST API for header queries
- **CDN Server** on port 3012 - Static header files for bootstrap (optional)

Configure via environment variables in `.env`:

```bash
# Copy the example config
cp env.docker .env

# Key settings:
CHAIN=main                                          # Network: main or test
CHAINTRACKS_MODE=embedded                           # Mode: embedded or remote
CDN_ENABLED=true                                    # Enable CDN server on port 3012
BOOTSTRAP_URL=https://chaintracks-cdn-us-1.bsvb.tech  # Bootstrap source
BOOTSTRAP_MODE=cdn                                  # Bootstrap mode: cdn or api
```

</details>

<details>
<summary><strong><code>API Endpoints</code></strong></summary>

**API Server (port 3011):**
- `GET /v2/network` - Network name (main or test)
- `GET /v2/tip` - Chain tip header (JSON)
- `GET /v2/tip.bin` - Chain tip header (binary)
- `GET /v2/tip/stream` - SSE stream for real-time tip updates
- `GET /v2/header/height/:height` - Header by height (JSON)
- `GET /v2/header/height/:height.bin` - Header by height (binary)
- `GET /v2/header/hash/:hash` - Header by hash (JSON)
- `GET /v2/header/hash/:hash.bin` - Header by hash (binary)
- `GET /v2/headers?height=N&count=C` - Multiple headers (binary)

**CDN Server (port 3012, when enabled):**
- `GET /{network}NetBlockHeaders.json` - Metadata with file list
- `GET /{network}Net_{index}.headers` - Binary header files (100k headers each)
- `GET /health` - CDN health check

Full API documentation available at `/docs` when running.

</details>

<details>
<summary><strong><code>Data Storage</code></strong></summary>

Headers are stored in 100k-block files:
```text
~/.chaintracks/
├── mainNetBlockHeaders.json    # Metadata
├── mainNet_0.headers           # Blocks 0-99999
├── mainNet_1.headers           # Blocks 100000-199999
└── ...
```

Each header is 80 bytes. Files use seek-based updates for efficient writes.

This format is compatible with the TypeScript chaintracks-server CDN format, allowing:
- Bootstrap from any CDN hosting these files
- Self-hosting your own CDN for other nodes to bootstrap from

</details>

<details>
<summary><strong><code>Bootstrap Options</code></strong></summary>

Two bootstrap modes are supported:

**CDN Mode (recommended):**
```bash
BOOTSTRAP_URL=https://chaintracks-cdn-us-1.bsvb.tech
BOOTSTRAP_MODE=cdn
```
Downloads headers from TypeScript CDN format files. Fast and efficient for initial sync.

**API Mode:**
```bash
BOOTSTRAP_URL=https://mainnet.gorillanode.io/api/v1
BOOTSTRAP_MODE=api
```
Fetches headers via REST API calls. Compatible with gorillanode-style endpoints.

</details>

<details>
<summary><strong><code>Development Build Commands</code></strong></summary>
<br/>

Get the [MAGE-X](https://github.com/mrz1836/mage-x) build tool for development:
```shell script
go install github.com/mrz1836/mage-x/cmd/magex@latest
```

View all build commands

```bash script
magex help
```

</details>

<details>
<summary><strong>Repository Features</strong></summary>
<br/>

This repository includes 25+ built-in features covering CI/CD, security, code quality, developer experience, and community tooling.

**[View the full Repository Features list →](.github/docs/repository-features.md)**

</details>

<details>
<summary><strong><code>Library Deployment</code></strong></summary>
<br/>

This project uses [goreleaser](https://github.com/goreleaser/goreleaser) for streamlined binary and library deployment to GitHub. To get started, install it via:

```bash
brew install goreleaser
```

The release process is defined in the [.goreleaser.yml](.goreleaser.yml) configuration file.


Then create and push a new Git tag using:

```bash
magex version:bump push=true bump=patch branch=master
```

This process ensures consistent, repeatable releases with properly versioned artifacts and citation metadata.

</details>

<details>
<summary><strong><code>Pre-commit Hooks</code></strong></summary>
<br/>

Set up the Go-Pre-commit System to run the same formatting, linting, and tests defined in [AGENTS.md](.github/AGENTS.md) before every commit:

```bash
go install github.com/mrz1836/go-pre-commit/cmd/go-pre-commit@latest
go-pre-commit install
```

The system is configured via modular env files in [`.github/env/`](.github/env/README.md) and provides 17x faster execution than traditional Python-based pre-commit hooks. See the [complete documentation](http://github.com/mrz1836/go-pre-commit) for details.

</details>

<details>
<summary><strong>GitHub Workflows</strong></summary>
<br/>

All workflows are driven by modular configuration in [`.github/env/`](.github/env/README.md) — no YAML editing required.

**[View all workflows and the control center →](.github/docs/workflows.md)**

</details>

<details>
<summary><strong><code>Updating Dependencies</code></strong></summary>
<br/>

To update all dependencies (Go modules, linters, and related tools), run:

```bash
magex deps:update
```

This command ensures all dependencies are brought up to date in a single step, including Go modules and any tools managed by [MAGE-X](https://github.com/mrz1836/mage-x). It is the recommended way to keep your development environment and CI in sync with the latest versions.

</details>

<br/>

## 🧪 Examples & Tests

All unit tests run via [GitHub Actions](https://github.com/bsv-blockchain/go-chaintracks/actions) and use [Go version 1.26.x](https://go.dev/doc/go1.26). View the [configuration file](.github/workflows/fortress.yml).

Run all tests (fast):

```bash script
magex test
```

Run all tests with race detector (slower):
```bash script
magex test:race
```

<br/>

## ⚡ Benchmarks

Run the Go benchmarks:

```bash script
magex bench
```

<br/>

## 🛠️ Code Standards
Read more about this Go project's [code standards](.github/CODE_STANDARDS.md).

<br/>

## 🤖 AI Usage & Assistant Guidelines
Read the [AI Usage & Assistant Guidelines](.github/tech-conventions/ai-compliance.md) for details on how AI is used in this project and how to interact with AI assistants.

<br/>

## 👥 Maintainers
| [<img src="https://github.com/icellan.png" height="50" alt="Siggi" />](https://github.com/icellan) | [<img src="https://github.com/galt-tr.png" height="50" alt="Galt" />](https://github.com/galt-tr) | [<img src="https://github.com/mrz1836.png" height="50" alt="MrZ" />](https://github.com/mrz1836) |
|:--------------------------------------------------------------------------------------------------:|:-------------------------------------------------------------------------------------------------:|:------------------------------------------------------------------------------------------------:|
|                                [Siggi](https://github.com/icellan)                                 |                                [Dylan](https://github.com/galt-tr)                                 |                                [MrZ](https://github.com/mrz1836)                                 |

<br/>

## 🤝 Contributing
View the [contributing guidelines](.github/CONTRIBUTING.md) and please follow the [code of conduct](.github/CODE_OF_CONDUCT.md).

### How can I help?
All kinds of contributions are welcome :raised_hands:!
The most basic way to show your support is to star :star2: the project, or to raise issues :speech_balloon:.

[![Stars](https://img.shields.io/github/stars/bsv-blockchain/go-chaintracks?label=Please%20like%20us&style=social&v=1)](https://github.com/bsv-blockchain/go-chaintracks/stargazers)

<br/>

## 📝 License

[![License](https://img.shields.io/badge/license-OpenBSV-blue?style=flat&logo=springsecurity&logoColor=white)](LICENSE)
