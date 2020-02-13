/*
* To try in Ace editor, copy and paste into the mode creator
* here : http://ace.c9.io/tool/mode_creator.html
*/

define(function(require, exports, module) {
"use strict";
var oop = require("../lib/oop");
var TextHighlightRules = require("./text_highlight_rules").TextHighlightRules;
/* --------------------- START ----------------------------- */
var HlbHighlightRules = function() {
this.$rules = {
"start" : [
   {
      "token" : "comment",
      "regex" : "(#.*)"
   },
   {
      "token" : "constant",
      "regex" : "((\\b(0(b|B|o|O|x|X)[a-fA-F0-9]+)\\b)|(\\b(0|[1-9][0-9]*)\\b)|(\\b(true|false)\\b))"
   },
   {
      "token" : "punctuation",
      "regex" : "(\")",
      "push" : "common__1"
   },
   {
      "token" : "entity.name.type",
      "regex" : "(\\bstring\\b|\\bint\\b|\\bbool\\b|\\bfs\\b|\\boption\\b)"
   },
   {
      "token" : ["keyword", "punctuation"],
      "regex" : "(\\b[a-zA-Z_][a-zA-Z0-9]*\\b)(\\()",
      "push" : "params"
   },
   {
      "token" : "invalid",
      "regex" : "(\\))"
   },
   {
      "token" : "punctuation",
      "regex" : "(\\{)",
      "push" : "block"
   },
   {
      "token" : "invalid",
      "regex" : "(\\})"
   },
   {
      defaultToken : "text",
   }
], 
"block" : [
   {
      "token" : "punctuation",
      "regex" : "(\\})",
      "next" : "pop"
   },
   {
      "token" : "comment",
      "regex" : "(#.*)"
   },
   {
      "token" : "constant",
      "regex" : "((\\b(0(b|B|o|O|x|X)[a-fA-F0-9]+)\\b)|(\\b(0|[1-9][0-9]*)\\b)|(\\b(true|false)\\b))"
   },
   {
      "token" : "punctuation",
      "regex" : "(\")",
      "push" : "common__1"
   },
   {
      "token" : "variable.language",
      "regex" : "(\\b(with|as|variadic)\\b)"
   },
   {
      "token" : ["entity.name.type", "text", "punctuation"],
      "regex" : "(\\bstring\\b|\\bint\\b|\\bbool\\b|\\bfs\\b|\\boption\\b)([\\t ]+)(\\{)",
      "push" : "block"
   },
   {
      "token" : "variable",
      "regex" : "(\\b((?!(scratch|image|resolve|http|checksum|chmod|filename|git|keepGitDir|local|includePatterns|excludePatterns|followPaths|generate|frontendInput|shell|run|readonlyRootfs|env|dir|user|network|security|host|ssh|secret|mount|target|localPath|uid|gid|mode|readonly|tmpfs|sourcePath|cache|mkdir|createParents|chown|createdTime|mkfile|rm|allowNotFound|allowWildcards|copy|followSymlinks|contentsOnly|unpack|createDestPath)\\b)[a-zA-Z_][a-zA-Z0-9]*\\b))"
   },
   {
      defaultToken : "text",
   }
], 
"common__1" : [
   {
      "token" : "punctuation",
      "regex" : "(\")",
      "next" : "pop"
   },
   {
      defaultToken : "text",
   }
], 
"params" : [
   {
      "token" : "punctuation",
      "regex" : "(\\))",
      "next" : "pop"
   },
   {
      "token" : "entity.name.type",
      "regex" : "(\\bstring\\b|\\bint\\b|\\bbool\\b|\\bfs\\b|\\boption\\b)"
   },
   {
      "token" : "variable",
      "regex" : "(\\b[a-zA-Z_][a-zA-Z0-9]*\\b)"
   },
   {
      defaultToken : "text",
   }
]
};
this.normalizeRules();
};
/* ------------------------ END ------------------------------ */
oop.inherits(HlbHighlightRules, TextHighlightRules);
exports.HlbHighlightRules = HlbHighlightRules;
});
