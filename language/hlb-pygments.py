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
            (u'(#.*)', bygroups(Comment.Single)),
            (u'((\\b(0(b|B|o|O|x|X)[a-fA-F0-9]+)\\b)|(\\b(0|[1-9][0-9]*)\\b)|(\\b(true|false)\\b))', bygroups(Name.Constant)),
            (u'(\")', bygroups(Punctuation), 'common__1'),
            (u'(<<[-~]?)([A-Z]+)', bygroups(Punctuation, Name.Constant), 'common__2'),
            (u'(\\bstring\\b|\\bint\\b|\\bbool\\b|\\bfs\\b|\\bgroup\\b|\\boption(?!::)\\b|\\boption::(?:copy|frontend|git|http|image|local|mkdir|mkfile|mount|rm|run|secret|ssh)\\b)', bygroups(Keyword.Type)),
            (u'(\\b[a-zA-Z_][a-zA-Z0-9]*\\b)(\\()', bygroups(Keyword, Punctuation), 'params'),
            (u'(\\))', bygroups(Generic.Error)),
            (u'(\\{)', bygroups(Punctuation), 'block'),
            (u'(\\})', bygroups(Generic.Error)),
            ('(\n|\r|\r\n)', String),
            ('.', String),
        ], 
        'block' : [
            (u'(#.*)', bygroups(Comment.Single)),
            (u'((\\b(0(b|B|o|O|x|X)[a-fA-F0-9]+)\\b)|(\\b(0|[1-9][0-9]*)\\b)|(\\b(true|false)\\b))', bygroups(Name.Constant)),
            (u'(\")', bygroups(Punctuation), 'common__1'),
            (u'(<<[-~]?)([A-Z]+)', bygroups(Punctuation, Name.Constant), 'common__2'),
            (u'(\\b(with|as|variadic)\\b)', bygroups(Name.Builtin)),
            (u'(\\bstring\\b|\\bint\\b|\\bbool\\b|\\bfs\\b|\\bgroup\\b|\\boption(?!::)\\b|\\boption::(?:copy|frontend|git|http|image|local|mkdir|mkfile|mount|rm|run|secret|ssh)\\b)(?:[\\t ]+)(\\{)', bygroups(Keyword.Type, Punctuation), 'block'),
            (u'(\\b((?!(allowEmptyWildcard|allowNotFound|allowWildcard|cache|checksum|chmod|chown|contentsOnly|copy|createDestPath|createParents|createdTime|dir|dockerLoad|dockerPush|download|downloadDockerTarball|downloadOCITarball|downloadTarball|env|excludePatterns|filename|followPaths|followSymlinks|format|forward|frontend|gid|git|host|http|id|ignoreCache|image|includePatterns|input|insecure|keepGitDir|local|localEnv|localPaths|locked|mkdir|mkfile|mode|mount|network|node|opt|parallel|private|readonly|readonlyRootfs|resolve|rm|run|sandbox|scratch|secret|security|shared|sourcePath|ssh|target|tmpfs|uid|unix|unpack|unset|user|value)\\b)[a-zA-Z_][a-zA-Z0-9]*\\b))', bygroups(Name.Variable)),
            ('(\n|\r|\r\n)', String),
            ('.', String),
        ], 
        'common__1' : [
            ('(\n|\r|\r\n)', String),
            ('.', String),
        ], 
        'common__2' : [
            ('(\n|\r|\r\n)', String),
            ('.', String),
        ], 
        'params' : [
            (u'(\\bstring\\b|\\bint\\b|\\bbool\\b|\\bfs\\b|\\bgroup\\b|\\boption(?!::)\\b|\\boption::(?:copy|frontend|git|http|image|local|mkdir|mkfile|mount|rm|run|secret|ssh)\\b)', bygroups(Keyword.Type)),
            (u'(\\b[a-zA-Z_][a-zA-Z0-9]*\\b)', bygroups(Name.Variable)),
            ('(\n|\r|\r\n)', String),
            ('.', String),
        ]
    }

