'fileTypes' : [
  'hlb'
]
'name' : 'hlb'
'patterns' : [
  {
    'include' : '#main'
  }
]
'scopeName' : 'source.hlb'
'uuid' : '88c38584-8b5f-45be-93a6-e2c9da5b6e3f'
'repository' : {
  'main' : {
    'patterns' : [
      {
        'include' : '#common'
      }
      {
        'match' : '(\\bstring\\b|\\bint\\b|\\bbool\\b|\\bfs\\b|\\boption\\b)'
        'name' : 'entity.name.type.hlb'
      }
      {
        'begin' : '(\\b[a-zA-Z_][a-zA-Z0-9]*\\b)(\\()'
        'beginCaptures' : {
          '1' : {
            'name' : 'keyword.hlb'
          }
          '2' : {
            'name' : 'punctuation.hlb'
          }
        }
        'patterns' : [
          {
            'include' : '#params'
          }
        ]
        'end' : '(\\))'
        'endCaptures' : {
          '1' : {
            'name' : 'punctuation.hlb'
          }
        }
      }
      {
        'match' : '(\\))'
        'name' : 'invalid.hlb'
      }
      {
        'begin' : '(\\{)'
        'beginCaptures' : {
          '1' : {
            'name' : 'punctuation.hlb'
          }
        }
        'patterns' : [
          {
            'include' : '#block'
          }
        ]
        'end' : '(\\})'
        'endCaptures' : {
          '1' : {
            'name' : 'punctuation.hlb'
          }
        }
      }
      {
        'match' : '(\\})'
        'name' : 'invalid.hlb'
      }
    ]
  }
  'block' : {
    'patterns' : [
      {
        'include' : '#common'
      }
      {
        'match' : '(\\b(with|as|variadic)\\b)'
        'name' : 'variable.language.hlb'
      }
      {
        'begin' : '(\\bstring\\b|\\bint\\b|\\bbool\\b|\\bfs\\b|\\boption\\b)([\\t\\x{0020}]+)(\\{)'
        'beginCaptures' : {
          '1' : {
            'name' : 'entity.name.type.hlb'
          }
          '2' : {
            'name' : 'text.hlb'
          }
          '3' : {
            'name' : 'punctuation.hlb'
          }
        }
        'patterns' : [
          {
            'include' : '#block'
          }
        ]
        'end' : '(\\})'
        'endCaptures' : {
          '1' : {
            'name' : 'punctuation.hlb'
          }
        }
      }
      {
        'match' : '(\\b((?!(scratch|image|resolve|http|checksum|chmod|filename|git|keepGitDir|local|includePatterns|excludePatterns|followPaths|generate|frontendInput|shell|run|readonlyRootfs|env|dir|user|network|security|host|ssh|secret|mount|target|localPath|uid|gid|mode|readonly|tmpfs|sourcePath|cache|mkdir|createParents|chown|createdTime|mkfile|rm|allowNotFound|allowWildcards|copy|followSymlinks|contentsOnly|unpack|createDestPath)\\b)[a-zA-Z_][a-zA-Z0-9]*\\b))'
        'name' : 'variable.hlb'
      }
    ]
  }
  'common' : {
    'patterns' : [
      {
        'match' : '(#.*)'
        'name' : 'comment.hlb'
      }
      {
        'match' : '((\\b(0(b|B|o|O|x|X)[a-fA-F0-9]+)\\b)|(\\b(0|[1-9][0-9]*)\\b)|(\\b(true|false)\\b))'
        'name' : 'constant.hlb'
      }
      {
        'begin' : '(")'
        'beginCaptures' : {
          '1' : {
            'name' : 'punctuation.hlb'
          }
        }
        'contentName' : 'text.hlb'
        'end' : '(")'
        'endCaptures' : {
          '1' : {
            'name' : 'punctuation.hlb'
          }
        }
      }
    ]
  }
  'common__1' : {
    'patterns' : [
    ]
  }
  'params' : {
    'patterns' : [
      {
        'match' : '(\\bstring\\b|\\bint\\b|\\bbool\\b|\\bfs\\b|\\boption\\b)'
        'name' : 'entity.name.type.hlb'
      }
      {
        'match' : '(\\b[a-zA-Z_][a-zA-Z0-9]*\\b)'
        'name' : 'variable.hlb'
      }
    ]
  }
}
