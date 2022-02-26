package llbutil

import (
	"github.com/alecthomas/participle/v2/lexer"
	"github.com/moby/buildkit/solver/pb"
)

func PositionFromLexer(pos lexer.Position) pb.Position {
	return pb.Position{
		Line:      int32(pos.Line),
		Character: int32(pos.Column),
	}
}
