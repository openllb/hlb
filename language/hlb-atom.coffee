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
        'match' : '(\\bstring\\b|\\bint\\b|\\bbool\\b|\\bfs\\b|\\bgroup\\b|\\boption(?!::)\\b|\\boption::(?:copy|frontend|git|http|image|local|mkdir|mkfile|mount|rm|run|secret|ssh|template)\\b)'
        'name' : 'entity.name.type.hlb'
      }
      {
        'match' : '(\\b(import|export|from|binds|as|with|variadic)\\b)'
        'name' : 'keyword.hlb'
      }
      {
        'begin' : '(\\b[a-zA-Z_][a-zA-Z0-9]*\\b)(\\()'
        'beginCaptures' : {
          '1' : {
            'name' : 'variable.hlb'
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
        'begin' : '((?:[\\t\\x{0020}]+)binds(?:[\\t\\x{0020}]+))(\\()'
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
      {
        'match' : '(\\b[a-zA-Z_][a-zA-Z0-9]*\\b)'
        'name' : 'variable.hlb'
      }
    ]
  }
  'binding' : {
    'patterns' : [
      {
        'match' : '(\\b[a-zA-Z_][a-zA-Z0-9]*\\b)((?:[\\t\\x{0020}]+))(\\b[a-zA-Z_][a-zA-Z0-9]*\\b)'
        'captures' : {
          '1' : {
            'name' : 'variable.language.hlb'
          }
          '2' : {
            'name' : 'punctuation.hlb'
          }
          '3' : {
            'name' : 'variable.hlb'
          }
        }
      }
    ]
  }
  'block' : {
    'patterns' : [
      {
        'include' : '#common'
      }
      {
        'match' : '((?:[\\t\\x{0020}]+)with(?:[\\t\\x{0020}]+))'
        'name' : 'keyword.hlb'
      }
      {
        'match' : '(as)((?:[\\t\\x{0020}]+))(\\b[a-zA-Z_][a-zA-Z0-9]*\\b)'
        'captures' : {
          '1' : {
            'name' : 'keyword.hlb'
          }
          '2' : {
            'name' : 'punctuation.hlb'
          }
          '3' : {
            'name' : 'variable.hlb'
          }
        }
      }
      {
        'begin' : '(binds)((?:[\\t\\x{0020}]+))(\\()'
        'beginCaptures' : {
          '1' : {
            'name' : 'keyword.hlb'
          }
          '2' : {
            'name' : 'punctuation.hlb'
          }
          '3' : {
            'name' : 'punctuation.hlb'
          }
        }
        'patterns' : [
          {
            'include' : '#binding'
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
        'begin' : '(\\bstring\\b|\\bint\\b|\\bbool\\b|\\bfs\\b|\\bgroup\\b|\\boption(?!::)\\b|\\boption::(?:copy|frontend|git|http|image|local|mkdir|mkfile|mount|rm|run|secret|ssh|template)\\b)((?:[\\t\\x{0020}]+))(\\{)'
        'beginCaptures' : {
          '1' : {
            'name' : 'entity.name.type.hlb'
          }
          '2' : {
            'name' : 'punctuation.hlb'
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
        'match' : '(\\b((?!(allowEmptyWildcard|allowNotFound|allowWildcard|cache|checksum|chmod|chown|contentsOnly|copy|createDestPath|createParents|createdTime|dir|dockerLoad|dockerPush|download|downloadDockerTarball|downloadOCITarball|downloadTarball|env|excludePatterns|filename|followPaths|followSymlinks|format|forward|frontend|gid|git|host|http|id|ignoreCache|image|includePatterns|input|insecure|keepGitDir|local|localEnv|localPaths|locked|mkdir|mkfile|mode|mount|network|node|opt|parallel|private|readonly|readonlyRootfs|resolve|rm|run|sandbox|scratch|secret|security|shared|sourcePath|ssh|stringField|target|template|tmpfs|uid|unix|unpack|unset|user|value)\\b)[a-zA-Z_][a-zA-Z0-9]*\\b))'
        'name' : 'variable.hlb'
      }
      {
        'match' : '(\\b[a-zA-Z_][a-zA-Z0-9]*\\b)'
        'name' : 'variable.language.hlb'
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
        'contentName' : 'string.hlb'
        'end' : '(")'
        'endCaptures' : {
          '1' : {
            'name' : 'punctuation.hlb'
          }
        }
      }
      {
        'begin' : '(<<[-\\x{007e}]?)([A-Z]+)'
        'beginCaptures' : {
          '1' : {
            'name' : 'punctuation.hlb'
          }
          '2' : {
            'name' : 'constant.hlb'
          }
        }
        'contentName' : 'string.hlb'
        'end' : '(\\2)'
        'endCaptures' : {
          '1' : {
            'name' : 'constant.hlb'
          }
        }
      }
    ]
  }
  'common__1' : {
    'patterns' : [
    ]
  }
  'common__2' : {
    'patterns' : [
    ]
  }
  'params' : {
    'patterns' : [
      {
        'match' : '(variadic)'
        'name' : 'keyword.hlb'
      }
      {
        'match' : '(\\bstring\\b|\\bint\\b|\\bbool\\b|\\bfs\\b|\\bgroup\\b|\\boption(?!::)\\b|\\boption::(?:copy|frontend|git|http|image|local|mkdir|mkfile|mount|rm|run|secret|ssh|template)\\b)'
        'name' : 'entity.name.type.hlb'
      }
      {
        'match' : '(\\b[a-zA-Z_][a-zA-Z0-9]*\\b)'
        'name' : 'variable.hlb'
      }
    ]
  }
}
