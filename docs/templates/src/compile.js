const fs = require('fs');
const Handlebars = require('handlebars');

Handlebars.registerHelper('eq', function(a, b) {
    return a == b;
});

var source = fs.readFileSync('./reference/reference.md', 'utf8');
var template = Handlebars.compile(source);

var context = JSON.parse(fs.readFileSync('./data/reference.json', 'utf8'));
var md = template(context);


if (!fs.existsSync('./dist')){
    fs.mkdirSync('./dist');
}
fs.writeFileSync('./dist/reference.md', md);
