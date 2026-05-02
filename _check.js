const fs = require('fs');
const lines = fs.readFileSync(process.argv[2], 'utf8').split('\n');
for (let i = 276; i < 283; i++) console.log((i+1) + ': ' + JSON.stringify(lines[i]));
