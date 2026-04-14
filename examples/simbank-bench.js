// Sleep-free SimBank benchmark: connect → login → account inquiry → disconnect.
// Measures pure extension/protocol throughput, not JS wait overhead.
// Run: ./k6 run -e BENCH_ITERS=50 examples/simbank-bench.js

import { TN3270 } from 'k6/x/tn3270';
import { check } from 'k6';

const HOST = __ENV.SIMBANK_HOST || 'localhost';
const PORT = parseInt(__ENV.SIMBANK_PORT || '2023');
const ITERS = parseInt(__ENV.BENCH_ITERS || '20');

export const options = {
    vus: 1,
    iterations: ITERS,
};

export default function () {
    const tn = TN3270();
    try {
        tn.connect(HOST, PORT, 10);
        tn.waitForField();

        tn.type('IBMUSER');
        tn.tab();
        tn.type('SYS1');
        tn.enter();
        tn.waitForField();

        // Navigate to BANK
        tn.pf(1);
        tn.waitForField();
        tn.clear();
        tn.waitForField();
        tn.type('BANK');
        tn.enter();
        tn.waitForField();

        // Account inquiry
        tn.pf(1);
        tn.waitForField();
        tn.type('123456789');
        tn.enter();
        tn.waitForField();

        const screen = tn.getScreenText();
        check(screen, {
            'account found': (s) => s.includes('Account Found'),
        });
    } finally {
        tn.disconnect();
    }
}
