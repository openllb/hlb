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
          rule /(\bstring\b|\bint\b|\bbool\b|\bfs\b|\boption\b)/, Keyword::Type
          rule /(\b[a-zA-Z_][a-zA-Z0-9]*\b)(\()/ do
            groups Keyword, Punctuation
            push :params
          end
          rule /(\))/, String
          rule /(\{)/, Punctuation, :block
          rule /(\})/, String
          rule /(\n|\r|\r\n)/, String
          rule /./, String
      end

      state:block do
          rule /(#.*)/, Comment::Single
          rule /((\b(0(b|B|o|O|x|X)[a-fA-F0-9]+)\b)|(\b(0|[1-9][0-9]*)\b)|(\b(true|false)\b))/, Name::Constant
          rule /(")/, Punctuation, :common__1
          rule /(\b(with|as|variadic)\b)/, Name::Builtin
          rule /(\bstring\b|\bint\b|\bbool\b|\bfs\b|\boption\b)([\t ]+)(\{)/ do
            groups Keyword::Type, String, Punctuation
            push :block
          end
          rule /(\b((?!(scratch|image|resolve|http|checksum|chmod|filename|git|keepGitDir|local|includePatterns|excludePatterns|followPaths|generate|frontendInput|shell|run|readonlyRootfs|env|dir|user|network|security|host|ssh|secret|mount|target|localPath|uid|gid|mode|readonly|tmpfs|sourcePath|cache|mkdir|createParents|chown|createdTime|mkfile|rm|allowNotFound|allowWildcards|copy|followSymlinks|contentsOnly|unpack|createDestPath)\b)[a-zA-Z_][a-zA-Z0-9]*\b))/, Name::Variable
          rule /(\n|\r|\r\n)/, String
          rule /./, String
      end

      state:common__1 do
          rule /(\n|\r|\r\n)/, String
          rule /./, String
      end

      state:params do
          rule /(\bstring\b|\bint\b|\bbool\b|\bfs\b|\boption\b)/, Keyword::Type
          rule /(\b[a-zA-Z_][a-zA-Z0-9]*\b)/, Name::Variable
          rule /(\n|\r|\r\n)/, String
          rule /./, String
      end

    end
  end
end

