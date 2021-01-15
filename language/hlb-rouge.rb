# -*- coding: utf-8 -*- #

module Rouge
  module Lexers
    class Hlb < RegexLexer
      title     "hlb"
      tag       'Hlb'
      mimetypes 'text/x-hlb'
      filenames '*.hlb'

      state:root do
          rule /(#.*)/, Comment::Single
          rule /((\b(0(b|B|o|O|x|X)[a-fA-F0-9]+)\b)|(\b(0|[1-9][0-9]*)\b)|(\b(true|false)\b))/, Name::Constant
          rule /(")/, Punctuation, :common__1
          rule /(<<[-~]?)([A-Z]+)/ do
            groups Punctuation, Name::Constant
            push :common__2
          end
          rule /(\bstring\b|\bint\b|\bbool\b|\bfs\b|\bgroup\b|\boption(?!::)\b|\boption::(?:copy|frontend|git|http|image|local|mkdir|mkfile|mount|rm|run|secret|ssh|template)\b)/, Keyword::Type
          rule /(\b(import|export|from|binds|as|with|variadic)\b)/, Keyword
          rule /(\b[a-zA-Z_][a-zA-Z0-9_]*\b)(\()/ do
            groups Name::Variable, Punctuation
            push :params
          end
          rule /(\))/, Generic::Error
          rule /((?:[\t ]+)binds(?:[\t ]+))(\()/ do
            groups Keyword, Punctuation
            push :params
          end
          rule /(\))/, Generic::Error
          rule /(\{)/, Punctuation, :block
          rule /(\})/, Generic::Error
          rule /(\b[a-zA-Z_][a-zA-Z0-9_]*\b)/, Name::Variable
          rule /(\n|\r|\r\n)/, String
          rule /./, String
      end

      state:binding do
          rule /(\b[a-zA-Z_][a-zA-Z0-9_]*\b)((?:[\t ]+))(\b[a-zA-Z_][a-zA-Z0-9_]*\b)/ do
            groups Name::Builtin, Punctuation, Name::Variable
          end
          rule /(\n|\r|\r\n)/, String
          rule /./, String
      end

      state:block do
          rule /(#.*)/, Comment::Single
          rule /((\b(0(b|B|o|O|x|X)[a-fA-F0-9]+)\b)|(\b(0|[1-9][0-9]*)\b)|(\b(true|false)\b))/, Name::Constant
          rule /(")/, Punctuation, :common__1
          rule /(<<[-~]?)([A-Z]+)/ do
            groups Punctuation, Name::Constant
            push :common__2
          end
          rule /((?:[\t ]+)with(?:[\t ]+))/, Keyword
          rule /(as)((?:[\t ]+))(\b[a-zA-Z_][a-zA-Z0-9_]*\b)/ do
            groups Keyword, Punctuation, Name::Variable
          end
          rule /(binds)((?:[\t ]+))(\()/ do
            groups Keyword, Punctuation, Punctuation
            push :binding
          end
          rule /(\bstring\b|\bint\b|\bbool\b|\bfs\b|\bgroup\b|\boption(?!::)\b|\boption::(?:copy|frontend|git|http|image|local|mkdir|mkfile|mount|rm|run|secret|ssh|template)\b)((?:[\t ]+))(\{)/ do
            groups Keyword::Type, Punctuation, Punctuation
            push :block
          end
          rule /(\b((?!(allowEmptyWildcard|allowNotFound|allowWildcard|cache|checksum|chmod|chown|contentsOnly|copy|createDestPath|createParents|createdTime|dir|dockerLoad|dockerPush|download|downloadDockerTarball|downloadOCITarball|downloadTarball|env|excludePatterns|filename|followPaths|followSymlinks|format|forward|frontend|gid|git|host|http|id|ignoreCache|image|includePatterns|input|insecure|keepGitDir|local|localEnv|localPaths|locked|mkdir|mkfile|mode|mount|network|node|opt|parallel|private|readonly|readonlyRootfs|resolve|rm|run|sandbox|scratch|secret|security|shared|sourcePath|ssh|stringField|target|template|tmpfs|uid|unix|unpack|unset|user|value)\b)[a-zA-Z_][a-zA-Z0-9]*\b))/, Name::Variable
          rule /(\b[a-zA-Z_][a-zA-Z0-9_]*\b)/, Name::Builtin
          rule /(\n|\r|\r\n)/, String
          rule /./, String
      end

      state:common__1 do
          rule /(\n|\r|\r\n)/, String
          rule /./, String
      end

      state:common__2 do
          rule /(\n|\r|\r\n)/, String
          rule /./, String
      end

      state:params do
          rule /(variadic)/, Keyword
          rule /(\bstring\b|\bint\b|\bbool\b|\bfs\b|\bgroup\b|\boption(?!::)\b|\boption::(?:copy|frontend|git|http|image|local|mkdir|mkfile|mount|rm|run|secret|ssh|template)\b)/, Keyword::Type
          rule /(\b[a-zA-Z_][a-zA-Z0-9_]*\b)/, Name::Variable
          rule /(\n|\r|\r\n)/, String
          rule /./, String
      end

    end
  end
end

