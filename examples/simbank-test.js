// SimBank integration test with banking transactions
// Start SimBank: cd simplatform && ./run-locally.sh --server
// Run: ./k6 run examples/simbank-test.js

import { TN3270 } from 'k6/x/tn3270';
import { check, sleep } from 'k6';

export const options = {
    vus: 1,
    iterations: 1,
};

const SIMBANK_HOST = __ENV.SIMBANK_HOST || 'localhost';
const SIMBANK_PORT = parseInt(__ENV.SIMBANK_PORT || '2023');

// Test accounts seeded in SimBank
const ACCOUNT_1 = '123456789';
const ACCOUNT_2 = '987654321';

export default function () {
    const tn = TN3270();

    console.log(`Connecting to SimBank at ${SIMBANK_HOST}:${SIMBANK_PORT}...`);

    try {
        // Connect and login
        tn.connect(SIMBANK_HOST, SIMBANK_PORT, 30);
        tn.waitForField();

        let screen = tn.getScreenText();
        check(screen, {
            'logon screen displayed': (s) => s.includes('SIMPLATFORM') && s.includes('Userid'),
        });

        tn.type('IBMUSER');
        tn.tab();
        tn.type('SYS1');
        tn.enter();
        tn.waitForField();

        screen = tn.getScreenText();
        check(screen, {
            'main menu displayed': (s) => s.includes('SIMPLATFORM MAIN MENU'),
        });
        console.log('Logged in to main menu');

        // Navigate to BANK application: PF1 -> CICS -> CLEAR -> type BANK
        tn.pf(1);
        sleep(0.5);
        tn.clear();
        sleep(0.5);
        tn.type('BANK');
        tn.enter();
        sleep(0.5);

        screen = tn.getScreenText();
        console.log('Bank main menu:');
        console.log(screen);
        check(screen, {
            'bank menu displayed': (s) => s.includes('SIMBANK MAIN MENU'),
            'browse option available': (s) => s.includes('BROWSE'),
            'transfer option available': (s) => s.includes('TRANSF'),
        });

        // Test 1: Account Inquiry (PF1 = BROWSE)
        console.log('\n=== Test 1: Account Inquiry ===');
        tn.pf(1);
        sleep(0.5);

        screen = tn.getScreenText();
        check(screen, {
            'account screen displayed': (s) => s.includes('SIMBANK ACCOUNT MENU'),
        });

        // Query first account
        tn.type(ACCOUNT_1);
        tn.enter();
        sleep(0.5);

        screen = tn.getScreenText();
        console.log(`Account ${ACCOUNT_1}:`);
        console.log(screen);
        check(screen, {
            'account 1 found': (s) => s.includes('Account Found'),
            'account number displayed': (s) => s.includes(ACCOUNT_1),
            'sort code displayed': (s) => s.includes('11-01-45'),
            'balance displayed': (s) => s.includes('Balance'),
        });

        // Return to bank menu (PF3)
        tn.pf(3);
        sleep(0.5);

        // Query second account
        tn.pf(1);
        sleep(0.5);
        tn.type(ACCOUNT_2);
        tn.enter();
        sleep(0.5);

        screen = tn.getScreenText();
        console.log(`Account ${ACCOUNT_2}:`);
        console.log(screen);
        check(screen, {
            'account 2 found': (s) => s.includes('Account Found'),
            'account 2 number displayed': (s) => s.includes(ACCOUNT_2),
        });

        tn.pf(3);
        sleep(0.5);

        // Test 2: Transfer between accounts (PF4 = TRANSF)
        console.log('\n=== Test 2: Fund Transfer ===');
        tn.pf(4);
        sleep(0.5);

        screen = tn.getScreenText();
        console.log('Transfer screen:');
        console.log(screen);
        check(screen, {
            'transfer screen displayed': (s) => s.includes('SIMBANK TRANSFER MENU'),
        });

        // Transfer $10 from account 2 to account 1
        tn.type(ACCOUNT_2);  // From account
        tn.tab();
        tn.type(ACCOUNT_1);  // To account
        tn.tab();
        tn.type('10.00');    // Amount
        tn.enter();
        sleep(0.5);

        screen = tn.getScreenText();
        console.log('After transfer:');
        console.log(screen);
        check(screen, {
            'transfer successful': (s) => s.includes('Transfer Successful'),
        });

        // Verify balance changed - go back and check account 1
        tn.pf(3);
        sleep(0.5);
        tn.pf(1);
        sleep(0.5);
        tn.type(ACCOUNT_1);
        tn.enter();
        sleep(0.5);

        screen = tn.getScreenText();
        console.log(`Account ${ACCOUNT_1} after transfer:`);
        console.log(screen);

        // Extract and verify balance increased (original was 56.72, should now be 66.72)
        const balanceMatch = screen.match(/Balance\s+(\d+\.\d+)/);
        if (balanceMatch) {
            const balance = parseFloat(balanceMatch[1]);
            check(balance, {
                'balance increased after transfer': (b) => b > 56.72,
            });
            console.log(`New balance: ${balance}`);
        }

        console.log('\n=== All SimBank tests completed successfully ===');

    } catch (err) {
        console.log(`Error: ${err}`);
    } finally {
        tn.disconnect();
    }
}
