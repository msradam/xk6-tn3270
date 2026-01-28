# xk6-tn3270

[![CI](https://github.com/msradam/xk6-tn3270/actions/workflows/ci.yml/badge.svg)](https://github.com/msradam/xk6-tn3270/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/msradam/xk6-tn3270)](https://goreportcard.com/report/github.com/msradam/xk6-tn3270)

TN3270 terminal emulation support for [k6](https://k6.io/) load testing.

This extension allows you to perform load testing against IBM mainframe applications that use the TN3270 protocol.

## Features

- TN3270/TN3270E protocol support via s3270
- Standard 3270 operations (PF/PA keys, Enter, Tab, Clear)
- Screen content reading and text search
- Screenshot capture for debugging
- Concurrent VU support with per-VU subprocess isolation

## Prerequisites

- [s3270](http://x3270.bgp.nu/) - The 3270 terminal emulator engine

  ```bash
  # macOS
  brew install x3270

  # Ubuntu/Debian
  apt-get install x3270

  # RHEL/CentOS
  yum install x3270-x11
  ```

- [xk6](https://github.com/grafana/xk6) - k6 extension builder

  ```bash
  go install go.k6.io/xk6/cmd/xk6@latest
  ```

## Installation

Build k6 with the xk6-tn3270 extension:

```bash
xk6 build --with github.com/msradam/xk6-tn3270
```

Or build from local source:

```bash
git clone https://github.com/msradam/xk6-tn3270.git
cd xk6-tn3270
make build
```

## Usage

```javascript
import { TN3270 } from 'k6/x/tn3270';

export default function() {
    const tn = TN3270();

    // Connect to mainframe
    tn.connect('mainframe.example.com', 23);
    tn.waitForField();

    // Login
    tn.type('USERID');
    tn.tab();
    tn.type('PASSWORD');
    tn.enter();
    tn.waitForField();

    // Navigate using PF keys
    tn.pf(3);  // PF3 to go back
    tn.waitForField();

    // Get screen content
    const screen = tn.getScreenText();
    console.log(screen);

    // Take a screenshot for debugging
    tn.screenshot('screenshots/login-screen.txt');

    // Disconnect
    tn.disconnect();
}
```

## API Reference

### Connection Management

| Method | Description |
|--------|-------------|
| `connect(host, port, timeout?)` | Establish TN3270 connection. Timeout in seconds (default: 30, max: 300) |
| `disconnect()` | Clean shutdown of connection |
| `isConnected()` | Returns true if currently connected |

### Basic Input

| Method | Description |
|--------|-------------|
| `type(text)` | Type text at current cursor position (max 1920 chars) |
| `string(text)` | Alias for type() |
| `enter()` | Send Enter key |
| `tab()` | Send Tab key |
| `clear()` | Send Clear key |

### Function Keys

| Method | Description |
|--------|-------------|
| `pf(key)` | Send PF key (1-24) |
| `pa(key)` | Send PA key (1-3) |

### Screen Operations

| Method | Description |
|--------|-------------|
| `waitForField(timeout?)` | Wait for screen to be ready for input. Timeout in seconds (default: 30) |
| `waitForText(text, timeout?)` | Wait until text appears on screen |
| `getScreenText()` | Return current screen contents as text |
| `ascii()` | Alias for getScreenText() |

### Composite Operations

| Method | Description |
|--------|-------------|
| `sendCommand(command, wait?)` | Type text + Enter + optional wait for field (default: true) |
| `sendPF(key, wait?)` | Send PF key + optional wait for field (default: true) |

### Debugging

| Method | Description |
|--------|-------------|
| `screenshot(path)` | Save current screen to a text file (similar to k6 browser's screenshot) |
| `printScreen()` | Return formatted screen with line numbers and border for console output |

## Examples

See [examples/simbank-test.js](examples/simbank-test.js) for a smoke test against [Galasa SimBank](https://galasa.dev/docs/running-simbank-tests/).

## Running Tests

```bash
# Run unit tests
go test -v ./...

# Start Galasa SimBank for E2E testing
git clone https://github.com/galasa-dev/simplatform.git
cd simplatform && ./build-locally.sh && ./run-locally.sh --server

# Run smoke test against SimBank (port 2023)
make test-simbank
```

## How It Works

This extension uses [s3270](http://x3270.bgp.nu/), the scripting version of the x3270 terminal emulator, as the underlying TN3270 engine. Each k6 VU (Virtual User) spawns its own s3270 subprocess, allowing for concurrent connections to mainframe systems.

The architecture follows the pattern used by other k6 extensions like xk6-browser:

```
k6 VU → xk6-tn3270 (Go) → s3270 subprocess → Mainframe
```

## License

MIT License - see [LICENSE](LICENSE) file.
