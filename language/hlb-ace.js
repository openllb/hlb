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
               "token" : ["punctuation", "constant"],
               "regex" : "(<<[-~]?)([A-Z]+)",
               "push" : "common__2"
            },
            {
               "token" : "entity.name.type",
               "regex" : "(\\bstring\\b|\\bint\\b|\\bbool\\b|\\bfs\\b|\\bgroup\\b|\\boption(?!::)\\b|\\boption::(?:copy|frontend|git|http|image|local|mkdir|mkfile|mount|rm|run|secret|ssh|template)\\b)"
            },
            {
               "token" : "keyword",
               "regex" : "(\\b(import|export|from|as|with|variadic)\\b)"
            },
            {
               "token" : ["variable", "punctuation"],
               "regex" : "(\\b[a-zA-Z_][a-zA-Z0-9]*\\b)(\\()",
               "push" : "params"
            },
            {
               "token" : "invalid",
               "regex" : "(\\))"
            },
            {
               "token" : ["keyword", "punctuation"],
               "regex" : "((?:[\\t ]+)as(?:[\\t ]+))(\\()",
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
               "token" : "variable",
               "regex" : "(\\b[a-zA-Z_][a-zA-Z0-9]*\\b)"
            },
            {
               defaultToken : "text",
            }
         ],
         "binding" : [
            {
               "token" : "punctuation",
               "regex" : "(\\))",
               "next" : "pop"
            },
            {
               "token" : ["variable.language", "punctuation", "variable"],
               "regex" : "(\\b[a-zA-Z_][a-zA-Z0-9]*\\b)((?:[\\t ]+))(\\b[a-zA-Z_][a-zA-Z0-9]*\\b)"
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
               "token" : ["punctuation", "constant"],
               "regex" : "(<<[-~]?)([A-Z]+)",
               "push" : "common__2"
            },
            {
               "token" : "keyword",
               "regex" : "((?:[\\t ]+)with(?:[\\t ]+))"
            },
            {
               "token" : ["keyword", "punctuation", "variable"],
               "regex" : "(as)((?:[\\t ]+))(\\b[a-zA-Z_][a-zA-Z0-9]*\\b)"
            },
            {
               "token" : ["keyword", "punctuation", "punctuation"],
               "regex" : "(as)((?:[\\t ]+))(\\()",
               "push" : "binding"
            },
            {
               "token" : ["entity.name.type", "punctuation", "punctuation"],
               "regex" : "(\\bstring\\b|\\bint\\b|\\bbool\\b|\\bfs\\b|\\bgroup\\b|\\boption(?!::)\\b|\\boption::(?:copy|frontend|git|http|image|local|mkdir|mkfile|mount|rm|run|secret|ssh|template)\\b)((?:[\\t ]+))(\\{)",
               "push" : "block"
            },
            {
               "token" : "variable",
               "regex" : "(\\b((?!(allowEmptyWildcard|allowNotFound|allowWildcard|cache|checksum|chmod|chown|contentsOnly|copy|createDestPath|createParents|createdTime|dir|dockerLoad|dockerPush|download|downloadDockerTarball|downloadOCITarball|downloadTarball|env|excludePatterns|filename|followPaths|followSymlinks|format|forward|frontend|gid|git|host|http|id|ignoreCache|image|includePatterns|input|insecure|keepGitDir|local|localEnv|localPaths|locked|mkdir|mkfile|mode|mount|network|node|opt|parallel|private|readonly|readonlyRootfs|resolve|rm|run|sandbox|scratch|secret|security|shared|sourcePath|ssh|stringField|target|template|tmpfs|uid|unix|unpack|unset|user|value)\\b)[a-zA-Z_][a-zA-Z0-9]*\\b))"
            },
            {
               "token" : "variable.language",
               "regex" : "(\\b[a-zA-Z_][a-zA-Z0-9]*\\b)"
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
               defaultToken : "string",
            }
         ],
         "common__2" : [
            {
               "token" : "constant",
               "regex" : "(\\2)",
               "next" : "pop"
            },
            {
               defaultToken : "string",
            }
         ],
         "params" : [
            {
               "token" : "punctuation",
               "regex" : "(\\))",
               "next" : "pop"
            },
            {
               "token" : "keyword",
               "regex" : "(variadic)"
            },
            {
               "token" : "entity.name.type",
               "regex" : "(\\bstring\\b|\\bint\\b|\\bbool\\b|\\bfs\\b|\\bgroup\\b|\\boption(?!::)\\b|\\boption::(?:copy|frontend|git|http|image|local|mkdir|mkfile|mount|rm|run|secret|ssh|template)\\b)"
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
