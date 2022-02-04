package diagnostic

import (
	"bytes"
	"context"
	"fmt"
	"sort"
	"strings"
	"unicode"

	"github.com/alecthomas/participle/lexer"
	"github.com/logrusorgru/aurora"
)

type Type int

const (
	Primary Type = iota
	Secondary
)

type Span struct {
	Message string
	Type    Type
	Start   lexer.Position
	End     lexer.Position
}

type Option func(*SpanError)

func Spanf(t Type, start, end lexer.Position, format string, a ...interface{}) Option {
	return func(se *SpanError) {
		se.Spans = append(se.Spans, Span{
			Message: fmt.Sprintf(format, a...),
			Type:    t,
			Start:   start,
			End:     end,
		})
	}
}

func WithError(err error, pos, end lexer.Position, opts ...Option) error {
	se := &SpanError{
		Err: err,
		Pos: pos,
		End: end,
	}
	for _, opt := range opts {
		opt(se)
	}
	return se
}

type SpanError struct {
	Err      error
	Pos, End lexer.Position
	Spans    []Span
}

func (se *SpanError) Error() string {
	return fmt.Sprintf("%s %s", FormatPos(se.Pos), se.Err)
}

func (se *SpanError) Unwrap() error {
	return se.Err
}

type PrettyOption func(*PrettyInfo)

type PrettyInfo struct {
	NumContext int
}

func WithNumContext(num int) PrettyOption {
	return func(info *PrettyInfo) {
		info.NumContext = num
	}
}

func (se *SpanError) Pretty(ctx context.Context, opts ...PrettyOption) string {
	var (
		info    PrettyInfo
		reports []string
		sources = Sources(ctx)
		color   = Color(ctx)
	)
	for _, opt := range opts {
		opt(&info)
	}
	maxLn := se.maxLn(ctx, info.NumContext)
	gutter := strings.Repeat(" ", maxLn)

	filenames, spansByFilename := se.groupAnnnotations()
	for _, filename := range filenames {
		fb := sources.Get(filename)

		// Sort spans in the same module by line number.
		spans := spansByFilename[filename]

		if len(spans) == 0 {
			continue
		}

		sort.SliceStable(spans, func(i, j int) bool {
			return spans[i].Start.Line < spans[j].Start.Line
		})

		// Construct the header for this group of spans.
		pos := spans[0].Start
		if filename == se.Pos.Filename {
			pos = se.Pos
		}
		header := color.Sprintf(color.Underline("%s:%d:%d:"),
			pos.Filename, pos.Line, pos.Column,
		)

		var (
			sections []string
			prevLn   int
		)
		// Initialize the previous line number, this will be updated after every
		// span to determine how the next span render should join with the previous.
		// (i.e. if there's a gap or there's overlap).
		prevLn = spans[0].Start.Line - info.NumContext - 1
		if prevLn < 0 {
			prevLn = 0
		}

		for i, span := range spans {
			var (
				underline string
				msgColor  func(interface{}) aurora.Value
			)
			switch span.Type {
			case Primary:
				underline = "^"
				msgColor = color.Red
			case Secondary:
				underline = "-"
				msgColor = color.Green
			}

			data, err := fb.Line(span.Start.Line - 1)
			if err != nil {
				reports = append(reports, err.Error())
				continue
			}

			// Calculate padding for the underline and message.
			end := span.Start.Column - 1
			padding := bytes.Map(func(r rune) rune {
				if unicode.IsSpace(r) {
					return r
				}
				return ' '
			}, data[:end])

			before := span.Start.Line - info.NumContext
			if before < 1 {
				before = 1
			}
			if before < prevLn+1 {
				before = prevLn + 1
			}

			var (
				lines []string
				start int
			)
			if i == 0 && info.NumContext == 0 {
				lines = append(lines, color.Sprintf(color.Blue("%s │ "), strings.Repeat(" ", maxLn)))
				start += 1
			} else if before-prevLn > 1 {
				// If the next span is more than one line away from the previous,
				// connect with a triple dot.
				lines = append(lines, color.Sprintf(color.Blue("%s ⫶"), gutter))
				start += 1
			}

			// Add lines of leading context.
			for i := before; i < span.Start.Line; i++ {
				leading, err := fb.Line(i - 1)
				if err != nil {
					lines = append(lines, err.Error())
					continue
				}
				lines = append(lines, string(leading))
			}

			// Add line for the span.
			lines = append(lines, string(data))
			lines = append(lines, color.Sprintf(msgColor("%s%s"), padding, strings.Repeat(underline, span.End.Column-span.Start.Column)))

			// Offset is the number of lines taken by the underline and message.
			offset := 1
			if len(span.Message) > 0 {
				messageLines := strings.Split(span.Message, "\n")
				offset = len(messageLines) + 1
				for _, line := range messageLines {
					lines = append(lines, color.Sprintf("%s%s", padding, msgColor(line)))
				}
			}

			// Add lines of trailing context.
			after := span.Start.Line + info.NumContext + 1
			if after > fb.Len() {
				after = fb.Len()
			}
			if i < len(spans)-1 {
				nextBefore := spans[i+1].Start.Line - info.NumContext
				if nextBefore <= span.Start.Line {
					after = span.Start.Line + 1
				}
			}
			for i := span.Start.Line + 1; i < after; i++ {
				trailing, err := fb.Line(i - 1)
				if err != nil {
					lines = append(lines, err.Error())
					continue
				}
				lines = append(lines, string(trailing))
			}

			// Add line numbers.
			for j := start; j < len(lines); j++ {
				var ln string
				index := j - start
				if index <= span.Start.Line-before {
					// Add line numbers before the underline & message.
					ln = fmt.Sprintf("%d", before+index)
				} else if index > span.Start.Line-before+offset {
					// Add line numbers after the underline & message.
					ln = fmt.Sprintf("%d", before+index-offset)
				}
				prefix := color.Sprintf(color.Blue("%s%s │ "), ln, strings.Repeat(" ", maxLn-len(ln)))
				lines[j] = fmt.Sprintf("%s%s", prefix, lines[j])
			}

			sections = append(sections, strings.Join(lines, "\n"))
			prevLn = after - 1
		}

		body := strings.Join(sections, color.Sprintf(color.Blue("\n")))
		reports = append(reports, fmt.Sprintf("%s\n%s", header, body))
	}

	var title string
	if se.Err != nil {
		title = color.Sprintf(
			"%s: %s\n",
			color.Bold(color.Red("error")),
			color.Bold(se.Err),
		)
	}
	return fmt.Sprintf("%s%s", title, strings.Join(reports, "\n"))
}

func (se *SpanError) maxLn(ctx context.Context, numContext int) int {
	maxLn := 0
	for _, span := range se.Spans {
		fb := Sources(ctx).Get(span.Start.Filename)
		line := span.Start.Line + numContext
		if line > fb.Len() {
			line = fb.Len()
		}
		ln := fmt.Sprintf("%d", line)
		if len(ln) > maxLn {
			maxLn = len(ln)
		}
	}
	return maxLn
}

func (se *SpanError) groupAnnnotations() (filenames []string, spansByFilename map[string][]Span) {
	spansByFilename = make(map[string][]Span)
	for _, span := range se.Spans {
		spansByFilename[span.Start.Filename] = append(spansByFilename[span.Start.Filename], span)
	}
	for filename := range spansByFilename {
		if filename == se.Pos.Filename {
			continue
		}
		filenames = append(filenames, filename)
	}
	sort.Strings(filenames)
	return append([]string{se.Pos.Filename}, filenames...), spansByFilename
}

// FormatPos returns a lexer.Position formatted as a string.
func FormatPos(pos lexer.Position) string {
	return fmt.Sprintf("%s:%d:%d:", pos.Filename, pos.Line, pos.Column)
}

func Offset(pos lexer.Position, offset int, line int) lexer.Position { //nolint:unparam
	pos.Offset += offset
	pos.Column += offset
	pos.Line += line
	return pos
}
