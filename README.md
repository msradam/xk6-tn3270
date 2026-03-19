# xk6-tn3270

[![CI](https://github.com/msradam/xk6-tn3270/actions/workflows/ci.yml/badge.svg)](https://github.com/msradam/xk6-tn3270/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/msradam/xk6-tn3270)](https://goreportcard.com/report/github.com/msradam/xk6-tn3270)

A [k6](https://k6.io/) extension for load testing IBM mainframe applications over the TN3270 protocol.

## Features

- Native Go TN3270 (RFC 1576) and TN3270E (RFC 2355) protocol support — no external dependencies
- TLS/SSL support for production mainframe connections
- EBCDIC Code Page 037 and 1047 encoding
- Terminal models 2-5 with alternate screen sizes (24×80 to 27×132)
- Full 3270 data stream processing (Write, Erase/Write, WSF, SBA, SF, SFE, SA, MF, RA, EUA, GE, etc.)
- Extended attribute tracking (color, highlighting, character set per position)
- Reply mode support (field, extended field, character) with proper Read Buffer/Modified responses
- TN3270E RESPONSES function negotiation
- Standard 3270 operations (PF/PA keys, Enter, Tab, BackTab, Clear, Home, MoveCursor)
- Screen content reading, text search, and screenshot capture
- Concurrent VU support

## Prerequisites

- [xk6](https://github.com/grafana/xk6) — k6 extension builder

  ```bash
  go install go.k6.io/xk6/cmd/xk6@latest
  ```

## Installation

Build k6 with the extension:

```bash
xk6 build --with github.com/msradam/xk6-tn3270
```

Build from local source:

```bash
git clone https://github.com/msradam/xk6-tn3270.git
cd xk6-tn3270
make build
```

Build from a specific branch:

```bash
xk6 build --with github.com/msradam/xk6-tn3270@branch-name
```

Or point to a local checkout of any branch:

```bash
xk6 build --with github.com/msradam/xk6-tn3270=/path/to/local/xk6-tn3270
```

## Usage

```javascript
import { TN3270 } from 'k6/x/tn3270';

export default function() {
    const tn = TN3270();

    // Optional: configure before connecting
    // tn.setModel(5);            // 27×132 screen
    // tn.setCodePage('cp1047');   // z/OS code page

    tn.connect('mainframe.example.com', 23);
    // Or use TLS: tn.connectTLS('mainframe.example.com', 992, false);
    tn.waitForField();

    // Login
    tn.type('USERID');
    tn.tab();
    tn.type('PASSWORD');
    tn.enter();
    tn.waitForField();

    // Navigate
    tn.pf(3);
    tn.waitForField();

    // Read screen
    const screen = tn.getScreenText();
    console.log(screen);

    tn.disconnect();
}
```

## API Reference

### Configuration

| Method | Description |
|--------|-------------|
| `setModel(model)` | Set terminal model: 2 (24×80), 3 (32×80), 4 (43×80), 5 (27×132). Call before `connect()` |
| `setCodePage(name)` | Set EBCDIC code page: `"cp037"` (default), `"cp1047"` (z/OS). Call before `connect()` |

### Connection

| Method | Description |
|--------|-------------|
| `connect(host, port, timeout?)` | Connect to a TN3270 host. Timeout in seconds (default: 30, max: 300) |
| `connectTLS(host, port, insecure, timeout?)` | Connect over TLS. Set `insecure=true` to skip certificate verification |
| `disconnect()` | Close the connection |
| `isConnected()` | Returns `true` if connected |

### Input

| Method | Description |
|--------|-------------|
| `type(text)` | Type text at the cursor position (max 1920 chars) |
| `string(text)` | Alias for `type()` |
| `enter()` | Send Enter |
| `tab()` | Move to next unprotected field |
| `backTab()` | Move to previous unprotected field |
| `clear()` | Send Clear |
| `home()` | Move cursor to first unprotected field |
| `moveTo(row, col)` | Move cursor to position (1-based) |

### Function Keys

| Method | Description |
|--------|-------------|
| `pf(key)` | Send PF key (1–24) |
| `pa(key)` | Send PA key (1–3) |

### Screen

| Method | Description |
|--------|-------------|
| `waitForField(timeout?)` | Wait for an input field to be available. Timeout in seconds (default: 30) |
| `waitForText(text, timeout?)` | Wait until text appears on screen |
| `getScreenText()` | Return screen contents as text (24×80) |
| `ascii()` | Alias for `getScreenText()` |

### Composite

| Method | Description |
|--------|-------------|
| `sendCommand(command, wait?)` | Type text + Enter + optional wait (default: true) |
| `sendPF(key, wait?)` | Send PF key + optional wait (default: true) |

### Debugging

| Method | Description |
|--------|-------------|
| `screenshot(path)` | Save screen to a text file |
| `printScreen()` | Return formatted screen with line numbers and border |

## Examples

See [examples/simbank-test.js](examples/simbank-test.js) for a complete test against [Galasa SimBank](https://galasa.dev/docs/running-simbank-tests/).

## Running Tests

```bash
# Unit tests
go test -v ./...

# Unit tests with race detection
go test -race ./...
```

## Architecture

Each k6 VU gets its own native Go TN3270 emulator instance with a direct TCP connection to the mainframe:

```
k6 VU → xk6-tn3270 (Go, native TN3270/TN3270E) → Mainframe
```

## Building on z/OS

See [docs/zos-build.md](docs/zos-build.md) for z/OS USS build instructions.

## License

MIT — see [LICENSE](LICENSE).
