from pygments.lexer import RegexLexer, bygroups
from pygments.token import *

import re

__all__=['HlbLexer']

class HlbLexer(RegexLexer):
    name = 'Hlb'
    aliases = ['hlb']
    filenames = ['*.hlb']
    flags = re.MULTILINE | re.UNICODE

    tokens = {
        'root' : [
            (u'(#.*)', bygroups(String)),
            (u'((\\b(0(b|B|o|O|x|X)[a-fA-F0-9]+)\\b)|(\\b(0|[1-9][0-9]*)\\b)|(\\b(true|false)\\b))', bygroups(String)),
            (u'(\")', bygroups(String), 'common__1'),
            (u'(\\bstring\\b|\\bint\\b|\\bbool\\b|\\bfs\\b|\\boption\\b)', bygroups(String)),
            (u'(\\b[a-zA-Z_][a-zA-Z0-9]*\\b)(\\()', bygroups(String, String), 'params'),
            (u'(\\))', bygroups(String)),
            (u'(\\{)', bygroups(String), 'block'),
            (u'(\\})', bygroups(String)),
            ('(\n|\r|\r\n)', String),
            ('.', String),
        ], 
        'block' : [
            (u'(#.*)', bygroups(String)),
            (u'((\\b(0(b|B|o|O|x|X)[a-fA-F0-9]+)\\b)|(\\b(0|[1-9][0-9]*)\\b)|(\\b(true|false)\\b))', bygroups(String)),
            (u'(\")', bygroups(String), 'common__1'),
            (u'(\\b(with|as|variadic)\\b)', bygroups(String)),
            (u'(\\bstring\\b|\\bint\\b|\\bbool\\b|\\bfs\\b|\\boption\\b)([\\t ]+)(\\{)', bygroups(String, String, String), 'block'),
            (u'(\\b((?!(scratch|image|resolve|http|checksum|chmod|filename|git|keepGitDir|local|includePatterns|excludePatterns|followPaths|generate|frontendInput|shell|run|readonlyRootfs|env|dir|user|network|security|host|ssh|secret|mount|target|localPath|uid|gid|mode|readonly|tmpfs|sourcePath|cache|mkdir|createParents|chown|createdTime|mkfile|rm|allowNotFound|allowWildcards|copy|followSymlinks|contentsOnly|unpack|createDestPath)\\b)[a-zA-Z_][a-zA-Z0-9]*\\b))', bygroups(String)),
            ('(\n|\r|\r\n)', String),
            ('.', String),
        ], 
        'common__1' : [
            ('(\n|\r|\r\n)', String),
            ('.', String),
        ], 
        'params' : [
            (u'(\\bstring\\b|\\bint\\b|\\bbool\\b|\\bfs\\b|\\boption\\b)', bygroups(String)),
            (u'(\\b[a-zA-Z_][a-zA-Z0-9]*\\b)', bygroups(String)),
            ('(\n|\r|\r\n)', String),
            ('.', String),
        ]
    }

