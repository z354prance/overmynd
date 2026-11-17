const fs = require('node:fs');
const path = require('node:path');
const { isUtf8 } = require('node:buffer');

function check(directory) {
  for (const entry of fs.readdirSync(directory, { withFileTypes: true })) {
    const file = path.join(directory, entry.name);
    if (entry.isDirectory()) check(file);
    else if (/\.(html|css|js|svg|json)$/.test(file)) {
      const bytes = fs.readFileSync(file);
      if (!isUtf8(bytes) || bytes.toString('utf8').includes('\uFFFD')) {
        throw new Error(`Invalid UTF-8 or replacement character in ${file}`);
      }
    }
  }
}
check('web');
check('internal/webui/web');
console.log('PASS: web assets are valid UTF-8 without replacement characters');
