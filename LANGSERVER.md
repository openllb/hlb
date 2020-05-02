hlb langserver
==============

Language server for [hlb](https://github.com/openllb/hlb) speaking [LSP](https://github.com/Microsoft/language-server-protocol).

Capabilities
------------

| Capability            | Support |
|-----------------------|---------|
| Hover                 |    ✔    |
| Jump to definition    |    ✔    |
| Find references       |         |
| Completion            |         |
| Workspace symbols     |         |
| Semantic highlighting |    ✔    |

Installation
------------

To build and install the `hlb langserver` run:

```sh
go get -u github.com/openllb/hlb/cmd/hlb
```

Usage
-----

Kakoune ([kak-lsp](https://github.com/ul/kak-lsp/))
```toml
[language.hlb]
filetypes = ["hlb"]
roots = [".git", ".hg"]
command = "hlb-langserver"
offset_encoding = "utf-8"

[semantic_scopes]
# Map textmate scopes to kakoune faces for semantic highlighting
# the underscores are translated to dots, and indicate nesting.
# That is, if variable_other_field is omitted, it will try the face for
# variable_other and then variable
#
# To see a list of available scopes in the debug buffer, run lsp-semantic-available-scopes
string="string"
constant="value"
variable="variable"
keyword="keyword"
storage_modifier="type"
storage_type="type"
entity_name_function="function"
entity_name_namespace="module"
comment="comment"
```
