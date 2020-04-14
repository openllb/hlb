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
          rule /(\bstring\b|\bint\b|\bbool\b|\bfs\b|\boption(?!::)\b|\boption::(?:copy|frontend|git|http|image|local|mkdir|mkfile|mount|rm|run|secret|ssh)\b)/, Keyword::Type
          rule /(\b[a-zA-Z_][a-zA-Z0-9]*\b)(\()/ do
            groups Keyword, Punctuation
            push :params
          end
          rule /(\))/, Generic::Error
          rule /(\{)/, Punctuation, :block
          rule /(\})/, Generic::Error
          rule /(\n|\r|\r\n)/, String
          rule /./, String
      end

      state:block do
          rule /(#.*)/, Comment::Single
          rule /((\b(0(b|B|o|O|x|X)[a-fA-F0-9]+)\b)|(\b(0|[1-9][0-9]*)\b)|(\b(true|false)\b))/, Name::Constant
          rule /(")/, Punctuation, :common__1
          rule /(\b(with|as|variadic)\b)/, Name::Builtin
          rule /(\bstring\b|\bint\b|\bbool\b|\bfs\b|\boption(?!::)\b|\boption::(?:copy|frontend|git|http|image|local|mkdir|mkfile|mount|rm|run|secret|ssh)\b)(?:[\t ]+)(\{)/ do
            groups Keyword::Type, Punctuation
            push :block
          end
          rule /(\b((?!(allowEmptyWildcard|allowNotFound|allowWildcard|cache|checksum|chmod|chown|contentsOnly|copy|createDestPath|createParents|createdTime|dir|dockerLoad|dockerPush|download|downloadDockerTarball|downloadOCITarball|downloadTarball|env|excludePatterns|filename|followPaths|followSymlinks|format|forward|frontend|gid|git|host|http|id|ignoreCache|image|includePatterns|input|insecure|keepGitDir|local|localPaths|locked|mkdir|mkfile|mode|mount|network|node|opt|private|readonly|readonlyRootfs|resolve|rm|run|sandbox|scratch|secret|security|shared|sourcePath|ssh|target|tmpfs|uid|unix|unpack|unset|user|value)\b)[a-zA-Z_][a-zA-Z0-9]*\b))/, Name::Variable
          rule /(\n|\r|\r\n)/, String
          rule /./, String
      end

      state:common__1 do
          rule /(\n|\r|\r\n)/, String
          rule /./, String
      end

      state:params do
          rule /(\bstring\b|\bint\b|\bbool\b|\bfs\b|\boption(?!::)\b|\boption::(?:copy|frontend|git|http|image|local|mkdir|mkfile|mount|rm|run|secret|ssh)\b)/, Keyword::Type
          rule /(\b[a-zA-Z_][a-zA-Z0-9]*\b)/, Name::Variable
          rule /(\n|\r|\r\n)/, String
          rule /./, String
      end

    end
  end
end

