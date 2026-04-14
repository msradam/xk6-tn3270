// Probe a TN3270 host: attempt connect, wait for a field, dump screen, disconnect.
// Usage: ./k6 run -e HOST=pub400.com -e PORT=23 examples/public-3270-probe.js

import { TN3270 } from 'k6/x/tn3270';
import { check } from 'k6';

export const options = { vus: 1, iterations: 1 };

const HOST = __ENV.HOST || 'pub400.com';
const PORT = parseInt(__ENV.PORT || '23');
const TIMEOUT = parseInt(__ENV.TIMEOUT || '20');
const TLS = __ENV.TLS === '1';

export default function () {
    const tn = TN3270();
    console.log(`Probing ${TLS ? 'TLS ' : ''}${HOST}:${PORT}`);
    try {
        if (TLS) {
            tn.connectTLS(HOST, PORT, true, TIMEOUT);
        } else {
            tn.connect(HOST, PORT, TIMEOUT);
        }
        console.log(`Connected: isConnected=${tn.isConnected()}`);

        try {
            tn.waitForField(TIMEOUT);
            console.log('waitForField returned');
        } catch (e) {
            console.log(`waitForField failed: ${e}`);
        }

        const screen = tn.getScreenText();
        console.log('---SCREEN---');
        console.log(screen);
        console.log('---END---');
        check(screen, { 'screen non-empty': (s) => s && s.trim().length > 0 });
    } catch (err) {
        console.log(`Connection error: ${err}`);
    } finally {
        try { tn.disconnect(); } catch (e) {}
    }
}
