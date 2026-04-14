# xk6-tn3270

[![CI](https://github.com/msradam/xk6-tn3270/actions/workflows/ci.yml/badge.svg)](https://github.com/msradam/xk6-tn3270/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/msradam/xk6-tn3270)](https://goreportcard.com/report/github.com/msradam/xk6-tn3270)

A [k6](https://k6.io/) extension for load testing IBM mainframe applications over TN3270.

Native Go implementation of RFC 1576 (TN3270) and RFC 2355 (TN3270E) — no external `s3270`/`x3270` binary required. Behaviour matches the [Galasa zos3270](https://github.com/galasa-dev/galasa) Java terminal for single-VU iterations; verified against Galasa SimBank and a real Hercules MVS 3.8j system.

## Install

The native Go implementation lives on the `native` branch (tagged `v0.1.0`)
while it's in pilot. `main` still carries the older `s3270`-subprocess version.
Pin explicitly:

```bash
go install go.k6.io/xk6/cmd/xk6@latest
xk6 build --with github.com/msradam/xk6-tn3270@v0.1.0
# or track the branch:
xk6 build --with github.com/msradam/xk6-tn3270@native
```

For local development:

```bash
git clone -b native https://github.com/msradam/xk6-tn3270.git
cd xk6-tn3270
make build
```

## Usage

```javascript
import { TN3270 } from 'k6/x/tn3270';

export default function () {
    const tn = TN3270();
    tn.connect('mainframe.example.com', 23);
    tn.waitForField();

    tn.type('USERID');
    tn.tab();
    tn.type('PASSWORD');
    tn.enter();
    tn.waitForField();

    tn.pf(3);
    tn.waitForField();

    console.log(tn.getScreenText());
    tn.disconnect();
}
```

A complete Galasa SimBank test is at [examples/simbank-test.js](examples/simbank-test.js).

## API

### Configuration (call before `connect`)

| Method | Description |
|---|---|
| `setModel(2..5)` | Terminal model: 2 (24×80), 3 (32×80), 4 (43×80), 5 (27×132) |
| `setCodePage(name)` | EBCDIC code page: `"cp037"` (default) or `"cp1047"` |
| `setTrace(bool)` | Protocol-level debug log (passwords are masked) |

### Connection

| Method | Description |
|---|---|
| `connect(host, port, timeout?)` | Plain TN3270. Timeout in seconds, 1–300, default 30 |
| `connectTLS(host, port, insecure, timeout?)` | TLS with default policy (MinVersion 1.2, host as SNI) |
| `connectTLSWithOptions(host, port, options)` | TLS with `caCert`, `clientCert`/`clientKey`, `serverName`, `minVersion`, `cipherSuites`, `proxy`, `insecure`, `timeout` |
| `disconnect()` | Idempotent. Wipes the screen buffer in memory |
| `isConnected()` | Returns `true` while a session is open |

### Input

| Method | Description |
|---|---|
| `type(text)` / `string(text)` | Type at cursor (max 1920 chars). Auto-advances into the next unprotected field when the current field is exactly filled |
| `enter()`, `tab()`, `backTab()`, `home()`, `clear()` | Standard 3270 keys |
| `moveTo(row, col)` | 1-based cursor positioning |
| `pf(1..24)`, `pa(1..3)` | Function/program-attention keys |

### Screen

| Method | Description |
|---|---|
| `waitForField(timeout?)` | Block until keyboard unlocks and an input field exists |
| `waitForText(text, timeout?)` | Poll for text on screen |
| `getScreenText()` / `ascii()` | Decoded screen as a string. Non-display fields appear as spaces |
| `screenshot(path)` | Write the screen to a text file |
| `printScreen()` | Bordered screen with line numbers (debugging) |

### Composite

| Method | Description |
|---|---|
| `sendCommand(text, wait?)` | `type` + `enter` + optional `waitForField` |
| `sendPF(key, wait?)` | `pf(key)` + optional `waitForField` |

## TLS options

```javascript
tn.connectTLSWithOptions('mainframe.example.com', 992, {
    caCert: open('/etc/ssl/internal-ca.pem'),
    clientCert: open('/etc/ssl/client.pem'),
    clientKey: open('/etc/ssl/client.key'),
    minVersion: '1.3',
    cipherSuites: ['ECDHE-ECDSA-AES256-GCM-SHA384', 'ECDHE-ECDSA-CHACHA20-POLY1305'],
    proxy: 'socks5://bastion.internal:1080',
    timeout: 60,
});
```

`caCert`, `clientCert`, `clientKey` accept either inline PEM strings or filesystem paths. The cipher allow-list is restricted to AEAD + forward-secrecy suites; CBC, RC4 and non-PFS RSA suites are not selectable. `proxy` accepts `socks5://` and `socks5h://` URLs.

## Errors

Errors thrown to JS are objects with a stable `code` field. Branch on the code rather than message text:

```javascript
try {
    tn.connect('mainframe.example.com', 23, 5);
} catch (err) {
    if (err.code === 'connect_timeout') {
        // retry
    }
}
```

Codes: `invalid_argument`, `not_connected`, `connect_failed`, `connect_timeout`, `tls_handshake`, `negotiation_failed`, `wait_timeout`, `host_closed`, `protocol_error`, `init_context`, `protected_field`, `screenshot_failed`.

## Metrics

| Metric | Type |
|---|---|
| `tn3270_connect_duration` | Trend (ms) |
| `tn3270_send_duration` | Trend (ms) |
| `tn3270_wait_duration` | Trend (ms) |
| `tn3270_session_duration` | Trend (ms) |
| `tn3270_connects`, `tn3270_disconnects`, `tn3270_screens`, `tn3270_aids_sent`, `tn3270_errors`, `tn3270_wait_timeouts` | Counter |
| `tn3270_bytes_in`, `tn3270_bytes_out` | Counter (bytes) |

## Architecture

Each k6 VU gets its own emulator and TCP socket. Dials are routed through k6's `state.Dialer`, so `--blocked-hostnames`, DNS overrides, and the `Hosts` map all apply. If the JS script forgets `disconnect()`, a per-connection goroutine closes the socket when the VU iteration ends.

```
k6 VU ─▶ xk6-tn3270 (Go, native TN3270/TN3270E) ─▶ Mainframe
```

## Tests

```bash
go test ./...           # unit + integration
go test -race ./...     # race detector
xk6 lint .              # full extension compliance check
```

## License

MIT — see [LICENSE](LICENSE).
