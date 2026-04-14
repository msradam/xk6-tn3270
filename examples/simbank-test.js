// Galasa-canonical SimBank integration test. Mirrors the Java pattern from
// dev.galasa.simbank.manager.internal.SimBankTerminalImpl: after type() fills
// a fixed-width field, the cursor auto-advances into the next unprotected
// field, so explicit tab() calls between consecutive type() calls are omitted.
// This test should pass 15/15 on any emulator that implements IBM 3270
// field semantics faithfully (Galasa zos3270, x3270).

import { TN3270 } from 'k6/x/tn3270';
import { check, sleep } from 'k6';

export const options = {
    vus: 1,
    iterations: 1,
};

const SIMBANK_HOST = __ENV.SIMBANK_HOST || 'localhost';
const SIMBANK_PORT = parseInt(__ENV.SIMBANK_PORT || '2023');

const ACCOUNT_1 = '123456789';
const ACCOUNT_2 = '987654321';

export default function () {
    const tn = TN3270();

    try {
        tn.connect(SIMBANK_HOST, SIMBANK_PORT, 30);
        tn.waitForField();

        let screen = tn.getScreenText();
        check(screen, {
            'logon screen displayed': (s) => s.includes('SIMPLATFORM') && s.includes('Userid'),
        });

        // Userid (8) isn't fully filled by IBMUSER (7), so tab() is needed to
        // cross the field boundary — auto-advance only fires when the field
        // is exactly filled.
        tn.type('IBMUSER');
        tn.tab();
        tn.type('SYS1');
        tn.enter();
        tn.waitForField();

        screen = tn.getScreenText();
        check(screen, {
            'main menu displayed': (s) => s.includes('SIMPLATFORM MAIN MENU'),
        });

        tn.pf(1);
        sleep(0.5);
        tn.clear();
        sleep(0.5);
        tn.type('BANK');
        tn.enter();
        sleep(0.5);

        screen = tn.getScreenText();
        check(screen, {
            'bank menu displayed': (s) => s.includes('SIMBANK MAIN MENU'),
            'browse option available': (s) => s.includes('BROWSE'),
            'transfer option available': (s) => s.includes('TRANSF'),
        });

        tn.pf(1);
        sleep(0.5);
        screen = tn.getScreenText();
        check(screen, {
            'account screen displayed': (s) => s.includes('SIMBANK ACCOUNT MENU'),
        });

        tn.type(ACCOUNT_1);
        tn.enter();
        sleep(0.5);
        screen = tn.getScreenText();
        check(screen, {
            'account 1 found': (s) => s.includes('Account Found'),
            'account number displayed': (s) => s.includes(ACCOUNT_1),
            'sort code displayed': (s) => s.includes('11-01-45'),
            'balance displayed': (s) => s.includes('Balance'),
        });

        tn.pf(3);
        sleep(0.5);

        tn.pf(1);
        sleep(0.5);
        tn.type(ACCOUNT_2);
        tn.enter();
        sleep(0.5);
        screen = tn.getScreenText();
        check(screen, {
            'account 2 found': (s) => s.includes('Account Found'),
            'account 2 number displayed': (s) => s.includes(ACCOUNT_2),
        });

        tn.pf(3);
        sleep(0.5);

        tn.pf(4);
        sleep(0.5);
        screen = tn.getScreenText();
        check(screen, {
            'transfer screen displayed': (s) => s.includes('SIMBANK TRANSFER MENU'),
        });

        // 9-char "From Account" auto-advances into 9-char "To Account", which
        // auto-advances into "Transfer Amount" — no tab() needed between fills.
        tn.type(ACCOUNT_2);
        tn.type(ACCOUNT_1);
        tn.type('10.00');
        tn.enter();
        sleep(0.5);

        screen = tn.getScreenText();
        check(screen, {
            'transfer successful': (s) => s.includes('Transfer Successful'),
        });

        tn.pf(3);
        sleep(0.5);
        tn.pf(1);
        sleep(0.5);
        tn.type(ACCOUNT_1);
        tn.enter();
        sleep(0.5);

        screen = tn.getScreenText();
        const balanceMatch = screen.match(/Balance\s+(\d+\.\d+)/);
        if (balanceMatch) {
            const balance = parseFloat(balanceMatch[1]);
            check(balance, {
                'balance increased after transfer': (b) => b > 56.72,
            });
        }
    } finally {
        tn.disconnect();
    }
}
